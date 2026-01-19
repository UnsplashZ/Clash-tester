async function operator(proxies) {
  var res;
  var data;

  try {
    // ⚠️ 确保你的地址在容器内能访问
    res = await fetch('http://localhost:8080/tags.json?noCache=true');
    data = await res.json();
  } catch (e) {
    return proxies;
  }

  for (var i = 0; i < proxies.length; i++) {
    var proxy = proxies[i];
    var record = data[proxy.name];

    if (!record) continue;

    // 初始化 tags 数组
    if (!proxy.tags || !Array.isArray(proxy.tags)) {
      proxy.tags = [];
    }

    // 辅助函数：防止重复添加标签
    function addTag(tag) {
      if (proxy.tags.indexOf(tag) === -1) {
        proxy.tags.push(tag);
      }
    }

    // --- 1. AI 服务 ---
    if (record.openai?.available) addTag('AI-OpenAI');
    if (record.gemini?.available) addTag('AI-Gemini');
    if (record.claude?.available) addTag('AI-Claude');

    // --- 2. 流媒体服务 ---
    
    // Netflix: 区分完整解锁
    if (record.netflix?.available) {
      if (record.netflix.result === "Full") {
        addTag('Stream-NF(全)');
      } else {
        addTag('Stream-NF(自制)');
      }
    }

    // Disney+
    if (record.disney?.available) addTag('Stream-Disney');

    // YouTube Premium
    if (record.youtube?.available) {
      if (record.youtube.premium) {
        addTag('Stream-YTP');
      } else {
        addTag('Stream-YouTube');
      }
    }

    // Max
    if (record.max?.available) addTag('Stream-Max');

    // --- ⚠️ 关键提示：如何让标签生效？ ---
    // 如果你是通过 Mihomo 的正则 filter 来分流，
    // 单纯加 tags 字段 Mihomo 是看不到的，必须把标签写进名字里。
    // 如果你需要改名，请取消下面这几行的注释：
    
    /*
    if (proxy.tags.length > 0) {
      // 过滤出我们需要展示的标签（比如只展示 Stream 开头的）
      // const showTags = proxy.tags.filter(t => t.startsWith('Stream') || t.startsWith('AI'));
      
      // 或者全部展示
      const tagPrefix = proxy.tags.map(t => `[${t}]`).join("");
      
      // 防止重复改名
      if (!proxy.name.includes(tagPrefix)) {
        proxy.name = `${tagPrefix} ${proxy.name}`;
      }
    }
    */
  }

  return proxies;
}