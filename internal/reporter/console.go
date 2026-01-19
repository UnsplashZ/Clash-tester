package reporter

import (
	"Clash-tester/pkg/models"
	"fmt"
	"strings"
)

func PrintConsole(report models.TestReport) {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Printf("Clash AI Service Tester - Test Report\n")
	fmt.Printf("Test Time: %s\n", report.TestTime.Format("2006-01-02 15:04:05"))
	fmt.Printf("Source: %s\n", report.Source)
	fmt.Println(strings.Repeat("=", 80))

	fmt.Printf("\nTotal Nodes: %d | Tested: %d | At least one service available: %d\n\n",
		report.TotalNodes, report.TestedNodes, report.SuccessNodes)

	// 打印每个节点的结果
	for i, node := range report.Results {
		fmt.Printf("[%d] %s (%s - %s)\n", i+1, node.NodeName, node.NodeType, node.Server)

		fmt.Println("  [AI Services]")
		printServiceResult("OpenAI", node.Tests["openai"])
		printServiceResult("Gemini", node.Tests["gemini"])
		printServiceResult("Claude", node.Tests["claude"])
		
		fmt.Println("  [Streaming]")
		printStreamResult("Netflix", node.StreamTests["netflix"])
		printStreamResult("Disney+", node.StreamTests["disney"])
		printStreamResult("Youtube", node.StreamTests["youtube"])
		printStreamResult("HBO Max", node.StreamTests["max"])

		fmt.Println()
	}

	// 打印摘要
	fmt.Println(strings.Repeat("-", 80))
	fmt.Println("Summary:")
	
	fmt.Println("  [AI Services]")
	printSummaryLine("OpenAI", report.Summary.OpenAI)
	printSummaryLine("Gemini", report.Summary.Gemini)
	printSummaryLine("Claude", report.Summary.Claude)
	
	fmt.Println("  [Streaming]")
	printSummaryLine("Netflix", report.Summary.Streaming["netflix"])
	printSummaryLine("Disney+", report.Summary.Streaming["disney"])
	printSummaryLine("Youtube", report.Summary.Streaming["youtube"])
	printSummaryLine("HBO Max", report.Summary.Streaming["max"])
	
	fmt.Println(strings.Repeat("=", 80))
}

func printServiceResult(name string, test models.ServiceTest) {
	status := "✗"
	if test.Available {
		status = "✓"
	}

	info := fmt.Sprintf("    %s %-8s", status, name)
	if test.Available {
		info += fmt.Sprintf(" [%s] (%dms)", 
			test.Country, test.ResponseTime)
	} else {
		// info += fmt.Sprintf(" [Failed: %s]", test.Error) // 简化输出，不显示详细错误
		info += fmt.Sprintf(" [Failed]") 
	}

	fmt.Println(info)
}

func printStreamResult(name string, test models.StreamTest) {
	status := "✗"
	if test.Available {
		status = "✓"
	}

	info := fmt.Sprintf("    %s %-8s", status, name)
	if test.Available {
		region := test.Region
		if region == "" {
			region = "OK"
		}
		info += fmt.Sprintf(" [%s] (%dms)", region, test.ResponseTime)
	} else {
		info += fmt.Sprintf(" [Failed]")
	}

	fmt.Println(info)
}

func printSummaryLine(name string, summary models.ServiceSummary) {
	fmt.Printf("    %-8s: ✓ %-3d | ✗ %-3d | Countries: %v\n",
		name, summary.Available, summary.Unavailable, summary.Countries)
}