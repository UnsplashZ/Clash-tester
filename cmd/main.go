package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"Clash-tester/internal/config"
	"Clash-tester/internal/parser"
	"Clash-tester/internal/proxy"
	"Clash-tester/internal/reporter"
	"Clash-tester/internal/tester"
	"Clash-tester/pkg/models"
)

type Worker struct {
	ID         int
	Core       *proxy.MihomoCore
	ConfigPath string
}

func main() {
	// 命令行参数
	// mode := flag.String("mode", "cli", "Running mode: cli (server mode removed)") // Deprecated
	source := flag.String("source", "", "Subscription URL or local YAML file path")
	output := flag.String("output", "result", "Output directory for detailed results")
	mapOutput := flag.String("map-output", "", "Path to save tags.json (Map format for SubStore)")
	mihomoPath := flag.String("mihomo", "mihomo.exe", "Path to mihomo executable")
	workersCount := flag.Int("workers", 5, "Number of concurrent workers")
	flag.Parse()

	// 兼容环境变量 (Docker Cron 模式使用)
	if *source == "" {
		envSource := os.Getenv("SUB_URL")
		if envSource != "" {
			*source = envSource
		}
	}

	if *source == "" {
		log.Fatal("Please provide -source parameter or SUB_URL environment variable")
	}

	runCLI(*source, *output, *mapOutput, *mihomoPath, *workersCount)
}

func runCLI(source, output, mapOutput, mihomoPath string, workersCount int) {
	printBanner()

	// 1. 加载配置
	fmt.Printf("📥 Loading configuration from: %s\n", source)
	data, err := config.Load(config.LoaderConfig{
		Source:  source,
		Timeout: 30,
	})
	if err != nil {
		log.Fatalf("❌ Failed to load config: %v", err)
	}

	// 2. 解析节点
	fmt.Println("🔍 Parsing subscription...")
	nodes, err := parser.Parse(data)
	if err != nil {
		log.Fatalf("❌ Failed to parse config: %v", err)
	}

	fmt.Printf("✅ Found %d supported nodes\n\n", len(nodes))

	if len(nodes) == 0 {
		log.Fatal("❌ No supported nodes found")
	}

	// 3. 初始化 Workers
	fmt.Printf("🚀 Starting %d mihomo workers...\n", workersCount)
	workers := make([]*Worker, 0, workersCount)
	
	// 确保所有核心和临时文件最终都被清理
	defer func() {
		fmt.Println("\n🧹 Cleaning up resources...")
		for _, w := range workers {
			if w.Core != nil {
				w.Core.Stop()
			}
			if w.ConfigPath != "" {
				os.Remove(w.ConfigPath)
			}
		}
	}()

	for i := 0; i < workersCount; i++ {
		workerID := i + 1
		tempConfig := fmt.Sprintf("temp_worker_%d.yaml", workerID)
		
		port := 7890 + (i * 10)
		apiPort := 9090 + i

		if err := config.GenerateMihomoConfig(nodes, tempConfig, port, apiPort); err != nil {
			log.Fatalf("❌ Failed to generate config for worker %d: %v", workerID, err)
		}

		core := proxy.NewMihomoCore(mihomoPath, tempConfig, port, apiPort)
		if err := core.Start(); err != nil {
			log.Fatalf("❌ Failed to start worker %d: %v", workerID, err)
		}

		workers = append(workers, &Worker{
			ID:         workerID,
			Core:       core,
			ConfigPath: tempConfig,
		})
		fmt.Printf("  ✅ Worker %d started (Port: %d, API: %d)\n", workerID, port, apiPort)
	}
	
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 4. 并发测试
	report := models.TestReport{
		TestTime:   time.Now(),
		Source:     source,
		TotalNodes: len(nodes),
		Results:    make([]models.NodeTestResult, 0, len(nodes)),
	}

	// 通道定义
	jobs := make(chan models.ProxyNode, len(nodes))
	results := make(chan models.NodeTestResult, len(nodes))
	var wg sync.WaitGroup

	// 启动 Worker Goroutines
	for _, w := range workers {
		wg.Add(1)
		go func(worker *Worker) {
			defer wg.Done()
			for node := range jobs {
				// 切换节点
				if err := worker.Core.SwitchProxy(node.Name); err != nil {
					log.Printf("⚠️  [Worker %d] Failed to switch to %s: %v", worker.ID, node.Name, err)
					continue
				}

				// 等待生效
			time.Sleep(500 * time.Millisecond)

				// 测试
				result := tester.TestNode(node, worker.Core.GetProxyURL())
				results <- result
			}
		}(w)
	}

	// 投递任务
	for _, node := range nodes {
		jobs <- node
	}
	close(jobs)

	// 等待完成并关闭结果通道
	go func() {
		wg.Wait()
		close(results)
	}()

	// 5. 收集结果与进度显示
	processedCount := 0
	for result := range results {
		processedCount++
		report.Results = append(report.Results, result)
		report.TestedNodes++
		
		if tester.IsNodeSuccess(result) {
			report.SuccessNodes++
		}

		// 打印进度
		printProgress(processedCount, len(nodes), result)
	}

	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 6. 生成摘要
	report.Summary = tester.GenerateSummary(report.Results)

	// 7. 输出结果
	reporter.PrintConsole(report)

	// 保存详细报告
	if err := reporter.SaveJSON(report, output); err != nil {
		log.Printf("⚠️  Failed to save detailed JSON: %v", err)
	} else {
		fmt.Printf("\n💾 Detailed results saved to: %s/\n", output)
	}

	// 保存 Map 格式报告 (如果指定)
	if mapOutput != "" {
		if err := reporter.SaveTagMapJSON(report, mapOutput); err != nil {
			log.Printf("⚠️  Failed to save Map JSON: %v", err)
			os.Exit(1) // 重要：如果生成 tags.json 失败，应该返回非 0 退出码，以便 Cron 脚本感知
		} else {
			fmt.Printf("💾 Tag Map JSON saved to: %s\n", mapOutput)
		}
	}

	fmt.Println("\n✨ Test completed!")
}

func printBanner() {
	banner := `
╔═══════════════════════════════════════════════════════╗
║                                                       ║
║        Clash AI Service Tester v1.3                  ║
║        Cron Mode Ready                                
║                                                       ║
╚═══════════════════════════════════════════════════════╝
`
	fmt.Println(banner)
}

func printProgress(current, total int, result models.NodeTestResult) {
	status := "❌"
	if tester.IsNodeSuccess(result) {
		status = "✅"
	}
	
	// 组装简短信息
	openai := getServiceStatusShort(result.Tests["openai"])
	netflix := getStreamStatusShort(result.StreamTests["netflix"])
	disney := getStreamStatusShort(result.StreamTests["disney"])
	
	fmt.Printf("[%3d/%d] %s %-20s (Chat:%s NF:%s D+:%s)\n", 
		current, total, status, truncateString(result.NodeName, 20), openai, netflix, disney)
}

func getServiceStatusShort(test models.ServiceTest) string {
	if !test.Available {
		return "✗"
	}
	if test.Country != "" {
		return test.Country
	}
	return "✓"
}

func getStreamStatusShort(test models.StreamTest) string {
	if !test.Available {
		return "✗"
	}
	if test.Region != "" {
		return test.Region
	}
	return "✓"
}

func truncateString(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}
