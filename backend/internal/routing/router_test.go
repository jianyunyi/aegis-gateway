package routing

import "testing"

func TestHeuristicTier(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"", ""},                                  // 空 → 不干预
		{"你好", "cheap"},                           // 短问答 → 便宜档
		{"今天天气怎么样？", "cheap"},                    // 短 → 便宜档
		{"请帮我写一个 Go 函数处理文件上传", "strong"},       // 含编程关键词 → 强档
		{"这个 SQL 查询为什么这么慢", "strong"},          // 含 SQL → 强档（短句关键词优先）
		{"帮我解释一下分布式事务的实现原理", "strong"},      // 含解释 → 强档
		{"请分析一下当前市场趋势并给出建议，内容比较长，需要详细展开说明一下各个维度的情况，包括宏观经济、行业竞争、用户需求等多个角度，最后给出可执行的行动方案", "normal"}, // 长且无关键词 → 标准档
	}
	for _, c := range cases {
		if got := HeuristicTier(c.text); got != c.want {
			t.Errorf("HeuristicTier(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}
