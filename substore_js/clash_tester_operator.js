/**
 * @name Clash-Tester Tag Injector (Cron Mode)
 * @description 读取外部 JSON 结果 (Map 格式)，自动为节点添加 [AI][NF] 等流媒体标签
 * @version 2.0
 */

async function operator(proxies, targetPlatform, context) {
  // ⚠️ 配置：你的 clash-tester Nginx 地址
  // 必须指向 tags.json 文件路径
  const API_URL = "http://你的服务器IP:8080/tags.json"; 
  
  let tagsMap = {};
  
  try {
      console.log(`[Clash-tester] 正在获取测试结果: ${API_URL}`);
      const resp = await $http.get(API_URL);
      
      // 解析 JSON
      tagsMap = typeof resp.body === 'string' ? JSON.parse(resp.body) : resp.body;
      
      if (!tagsMap || Object.keys(tagsMap).length === 0) {
          console.log("[Clash-tester] 警告: 获取到的结果为空");
          return proxies;
      }
      
      console.log(`[Clash-tester] 成功获取 ${Object.keys(tagsMap).length} 个节点的测试结果`);

  } catch (e) {
      console.log(`[Clash-tester] ⚠️ 无法获取测试结果，将跳过打标: ${e.message || e}`);
      return proxies; // 容错：获取失败返回原列表
  }

  return proxies.map(p => {
      // 通过节点名称查找结果
      // 注意：如果节点名称有变更，可能无法匹配。建议保持订阅中的名称一致。
      const data = tagsMap[p.name];
      
      // 如果该节点没有测试记录，直接返回原节点
      if (!data) return p;

      let tags = [];

      // --- 1. AI 服务 ---
      if (data.openai?.available) tags.push("Chat");
      // else if (data.claude?.available) tags.push("Claude"); // 可选
      // else if (data.gemini?.available) tags.push("Gemini"); // 可选

      // --- 2. 流媒体 (Netflix) ---
      if (data.netflix?.available) {
          let nfTag = "NF";
          // 区分完整解锁与自制剧
          if (data.netflix.result === "Originals Only") nfTag = "NF(O)";
          
          // 可选：添加地区后缀 (如 "NF-US")
          // if (data.netflix.region) nfTag += `-${data.netflix.region}`;
          
          tags.push(nfTag);
      }

      // --- 3. YouTube ---
      if (data.youtube?.available) {
          tags.push("YT");
          // if (data.youtube.premium) tags.push("YTP"); // 如果检测了 Premium
      }
      
      // --- 4. Disney+ ---
      if (data.disney?.available) {
          tags.push("DP");
      }

      // --- 5. 修改名称 ---
      // 原始: "香港节点 01"
      // 修改: "[Chat|NF|YT] 香港节点 01"
      if (tags.length > 0) {
          const tagStr = `[${tags.join("|")}]`;
          
          // 避免重复添加 (简单的字符串检查)
          if (p.name.indexOf(tagStr) === -1) {
              // 检查是否已经有类似格式的标签，避免堆叠
              // 这里简单处理：直接加在最前面
              p.name = `${tagStr} ${p.name}`;
          }
      }

      return p;
  });
}