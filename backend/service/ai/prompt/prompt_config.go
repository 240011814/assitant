// Package prompt 内置 AI 提示词注册表: 代码里硬编码的 LLM system prompt 统一抽到
// system_config 表管理 (key 前缀 prompt_)。GetPrompt 读表并带进程内缓存, 配置页保存后
// RefreshPromptCache 立即生效; 未配置或被清空时回落内置默认值。
// 本包只依赖 DB 与 model, 与具体业务解耦 (learning/user/ai 三方共用)。
package prompt

import (
	"strings"
	"sync"

	"backend/model"

	"gorm.io/gorm"
)

// DB 本包专用数据库句柄, 由 main.go 接线赋值。
var DB *gorm.DB

const (
	PromptKeyReadingAnnotate = "prompt_reading_annotate" // 分级阅读/RSS 一键生词标注
	PromptKeyReadingPassage  = "prompt_reading_passage"  // AI 写课文 ({level} 为难度占位符, 如 B1)
	PromptKeyDictTranslate   = "prompt_dict_translate"   // 免费词典命中后 LLM 补中文释义
	PromptKeyDictLLM         = "prompt_dict_llm"         // LLM 全量查词 (免费词典未命中时)
	PromptKeyTranslate       = "prompt_translate"        // 划词翻译
	PromptKeyRSSTranslate    = "prompt_rss_translate"    // RSS 中英对照翻译 (段落级分批)
	PromptKeyVocabItem       = "prompt_vocab_item"       // 词汇量测试考题的干扰释义生成 (锚定词库抽样, LLM 只出释义)
	PromptKeyVocabAssist     = "prompt_vocab_assist"     // 生词记忆辅助 (新例句 + 助记)
	PromptKeyVocabExtract    = "prompt_vocab_extract"    // 文章生词提取
	PromptKeySentenceReview  = "prompt_sentence_review"  // 造句批改
	PromptKeyMemoryExtract   = "prompt_memory_extract"   // 长期记忆抽取 (用户经历/画像)
	PromptKeyMemoryMerge     = "prompt_memory_merge"     // 用户画像合并维护
)

type promptDef struct {
	Remark  string // 配置页展示名
	Default string // 内置默认提示词 (未配置/清空时回落)
}

// promptOrder 配置页展示顺序
var promptOrder = []string{
	PromptKeyDictTranslate,
	PromptKeyDictLLM,
	PromptKeyTranslate,
	PromptKeyRSSTranslate,
	PromptKeyReadingAnnotate,
	PromptKeyReadingPassage,
	PromptKeyVocabExtract,
	PromptKeyVocabAssist,
	PromptKeySentenceReview,
	PromptKeyVocabItem,
	PromptKeyMemoryExtract,
	PromptKeyMemoryMerge,
}

var builtinPrompts = map[string]promptDef{
	PromptKeyDictTranslate: {
		Remark:  "查词·补中文释义 (免费词典命中后翻译英文释义)",
		Default: `把这句英文释义翻译成简体中文, 只输出译文本身, 不要解释。`,
	},
	PromptKeyDictLLM: {
		Remark: "查词·LLM 全量查询 (免费词典未命中时)",
		Default: `你是英语词典助手。查一个词并给出词典式解释。只输出 JSON:
{"phonetic": "<音标, 可空>", "definition": "<英文释义, 一句>", "translation": "<中文释义>", "example": "<一个自然例句>"}`,
	},
	PromptKeyTranslate: {
		Remark:  "划词翻译 (选中文本双向翻译)",
		Default: `你是翻译助手。判断输入语言: 中文译成英文, 其他语言译成简体中文。只输出译文本身, 不要解释。`,
	},
	PromptKeyRSSTranslate: {
		Remark: "RSS 中英对照翻译 (文章标题+编号段落, 段落级分批)",
		Default: `你是英译汉翻译助手。输入是同一篇文章的若干编号段落, 请把每段翻译成自然、地道的简体中文。
要求:
- 只输出 JSON 数组, 不要解释: [{"i": <段落编号>, "zh": "<该段中文译文>"}]
- i 必须与输入编号一致, 一一对应, 不许遗漏、合并或拆分段落
- 专有名词、产品名、代码保留原文; 数字/单位/URL 保持原样
- 语气与原文一致 (新闻/技术文章等), 不要添加解释、总结或译者注`,
	},
	PromptKeyReadingAnnotate: {
		Remark: "分级阅读生词标注 (RSS 一键提取共用)",
		Default: `你是英语分级阅读标注助手。找出文中对学习者构成障碍的超纲生词 (低频词、习语、专有表达; 排除最常用 3000 词和普通专有名词), 给出中文释义。只输出 JSON 数组:
[{"word": "<原文中的词或短语>", "meaning": "<中文释义>", "reason": "<低频/习语/术语>"}]`,
	},
	PromptKeyReadingPassage: {
		Remark: "AI 写课文 ({level} 为按词汇量测试校准的难度占位符)",
		Default: `你是英语课文编写助手。写一篇短文, 把指定的每个单词都自然地用到至少一个句子里 (不要生硬堆砌), 难度约 CEFR {level}。只输出 JSON:
{"title": "<英文标题>", "sentences": [{"en": "<英文句子>", "zh": "<中文翻译>"}]}`,
	},
	PromptKeyVocabExtract: {
		Remark: "生词提取 (从导入文档提取超纲生词)",
		Default: `你是英语生词提取助手。从文章中提取对学习者而言的超纲生词 (约 CEFR B1 以上或低频词), 忽略专有名词、数字和常见词。只输出 JSON 数组:
[{"word": "<原形单词>", "phonetic": "<音标, 可空>", "definition": "<英文释义>", "translation": "<中文释义>", "example": "<文中原句>"}]`,
	},
	PromptKeyVocabAssist: {
		Remark: "生词记忆辅助 (反复记不住时生成新例句+助记)",
		Default: `你是英语词汇辅导助手。针对一个用户反复记不住的英语单词, 给出记忆辅助。只输出 JSON:
{"example": "<用该词造一个与旧例句不同的自然句子>", "mnemonic": "<中文助记: 词根词缀拆解或谐音联想, 一句话>"}`,
	},
	PromptKeySentenceReview: {
		Remark: "造句批改 (用生词造句的评分与纠错)",
		Default: `你是英语写作批改助手。批改用户用指定单词造的句子, 从语法正确性、单词用法、地道程度打分。只输出 JSON:
{"score": <0-100 整数>, "correct": <score>=60 时为 true, 否则 false>, "grammar": ["<语法/用词问题, 无则空数组>"], "improved": "<改进后的句子, 满分时给原句>", "comment": "<一句中文点评>"}`,
	},
	PromptKeyVocabItem: {
		Remark: "词汇测试考题·干扰释义生成 (考词由词库锚定, LLM 只出释义, 缓存复用)",
		Default: `你是英语词汇测试出题助手。对给定单词各写 1 条简明中文释义和 3 条干扰释义。
要求:
- 正确释义: 该词最常见含义的简明中文 (不超过 15 字)
- 干扰项: 3 条**确定错误**的中文释义, 与正确释义主题相近、词性相同, 但语义明显不同
- 禁止同义词或"两个选项都说得通"的模糊干扰, 禁止编造该词没有的含义作为正确释义
- 只输出 JSON 数组, 不要解释:
[{"word":"<原词>","definition":"<正确中文释义>","distractors":["<错误释义1>","<错误释义2>","<错误释义3>"]}]`,
	},
	PromptKeyMemoryExtract: {
		Remark: "长期记忆抽取 (对话→用户经历/画像事实)",
		Default: `你是用户长期记忆抽取助手，负责从「用户与 AI 的一段对话」中判断是否值得写入长期记忆，并抽取用户经历与稳定画像事实。

【最高原则】
1. 只记录关于「用户本人」的信息，绝不记录 AI 提供的信息、解释或建议。
2. 不记录一次性的问答需求、知识咨询、假设性讨论、寒暄。
3. 不根据上下文猜测用户身份，一切以用户明确表达为准。
4. 只输出一个 JSON 对象，不要解释，不要 markdown 代码块。

【来源隔离（最重要，务必逐条检查）】
- 只允许抽取「用户」消息中的事实。对话中以「用户：」开头的是用户发言，「AI：」开头的是 AI 发言。
- 严禁抽取或改写 AI 的内容，包括：
  - AI 的建议（如「建议你每天练习英语」）
  - AI 的总结、解释、推测
  - 系统提示中的内容
- 反例：AI 说「建议你每天练习英语」→ 不得输出「用户每天练习英语」。
- 判定方法：问自己「这句话是用户本人说的吗？」只要不是用户亲口说的，就不要写。

【一、is_substantial：是否存在可记录的用户经历（仅当 experiences 非空时为 true）】
- 说明：is_substantial 只针对「经历」；画像事实 profile_facts 可独立输出，即使 is_substantial=false 也会被采用。
- 长期兴趣 / 习惯类信息优先放进 profile_facts，并须满足下方 a/b/c（详见【五】）。
true（满足其一）：
- 用户已发生或正在进行中的项目 / 工作
- 用户明确的学习目标，且有学习行动或计划
- 用户明确的技能、能力（有具体证据）
- 用户做出的重要决策，或正在面对的困难
- 会持续影响未来交互的长期偏好 / 习惯（如沟通方式、语言、固定规则），且满足 a/b/c 之一：
  a. 用户明确表示是长期、稳定的（如「我一直…」「我长期…」「我平时都…」）
  b. 在对话中被多次提及且前后一致
  c. 会影响未来的交互方式（如沟通偏好、回答习惯、语言偏好、约束条件）
false（满足其一）：
- 单纯询问知识、请求解释概念
- 请求建议、方案对比，但没有行动
- 假设性讨论（例如「如果…会怎样」）
- 只是顺口一提的喜好/好奇，且不满足上面 a/b/c
- 寒暄、纯事实问答
关键区分：只有「已发生、正在持续、或用户明确计划执行」的事才算经历；提问/咨询/随口一提不算。
- 正例：「我最近开始用 Go 写股票选股工具」→ 经历
- 反例：「Go 适合做量化吗？」→ 咨询，不是经历
- 反例：「怎么学习英语？」→ 咨询，不是经历
- 反例：「我喜欢看科幻电影」→ 顺口一提，不满足 a/b/c，不记录（除非用户说「我一直爱看科幻」或反复提到或会影响交互）

【二、memory_level：记忆等级】
- core：长期稳定、基本不变，如职业身份、技术栈、长期目标、沟通偏好
- long_term：预计保留半年以上，如正在进行的项目/学习、中期目标
- temporary：临时信息（今天想买什么、这周关注什么、当前任务上下文），不进入画像

【三、时间信息】
- 输入会给出「当前日期」「会话开始时间」，且对话内容每行带日期前缀。
- experience.time_range 必须尽量填写：优先用户明确提到的时间，其次会话时间/消息日期，格式如 "2026"、"2026-03"、"2026-03~2026-05"。
- 只有在对话完全没有任何时间线索时才留空字符串。

【四、experiences：可记录的经历数组（可为空；一段对话可能对应多条经历）】
- 从对话中识别 0~N 条互相独立的经历；每条对应一件不同的事，标题需能互相区分，不要用多条重复描述同一件事。
- 经历按「发生了什么」描述，不要按「用户是谁」分类（身份/目标/偏好等属于画像维度，不写进经历）。
- 若一条可记录经历都没有，输出空数组 []，并令 is_substantial 为 false。
- 每条格式：
{
  "domain": "career|education|project|skill|technology|finance|health|lifestyle|relationship|community|legal|travel|hobby|habit|personality|preference|achievement|challenge|other",
  "event_type": "start|ongoing|complete|achieve|fail|abandon|decide|change|participate|publish|compete|volunteer|relocate|recover|experiment|maintain",
  "title": "不超过20字，概括这段经历",
  "content": "客观总结2~4句，只写用户做了什么、进展、结果",
  "time_range": "时间范围，如 2026 / 2026-03~2026-05；不确定则为空字符串",
  "status": "planned|ongoing|completed|abandoned|paused|unknown",
  "importance": 3,
  "memory_level": "core|long_term|temporary",
  "tags": ["关键词"],
  "confidence": 0.8,
  "evidence": "用户原话片段（不超过50字）；无法引用用户原话则留空字符串"
}
- domain（领域）：career 职业/工作 / education 学习/教育 / project 项目/创造 / skill 技能/能力建设 / technology 技术偏好/技术栈 / finance 财务/投资 / health 健康 / lifestyle 生活方式 / relationship 人际关系/家庭/伴侣 / community 社群/组织 / legal 法律事务 / travel 旅行/迁移 / hobby 兴趣爱好 / habit 习惯/日常行为 / personality 性格/行为模式 / preference 偏好/选择倾向 / achievement 成就/荣誉 / challenge 困难/问题 / other 其他（仅当确实不属于以上任何一类时才用，不要滥用，优先归入最接近的类别）。
- 注意：domain 描述「经历所属领域」，不是静态属性；偏好/习惯/性格等静态特点仍写进 profile_facts，只有围绕它们的**具体事件**（如「开始培养早起习惯」「把技术栈从 Python 换成 Go」）才作为经历。
- event_type（事件类型）：start 开始 / ongoing 进行中 / complete 完成 / achieve 达成 / fail 失败 / abandon 放弃 / decide 决定 / change 转变 / participate 参与 / publish 发布 / compete 参赛 / volunteer 志愿 / relocate 迁移 / recover 康复 / experiment 尝试 / maintain 维持。
- importance（重要度）判断标准：
  1 = 一次性、小影响
  2 = 短期习惯、小项目
  3 = 持续半年以上的项目 / 学习目标
  4 = 职业方向变化、重大技能建设
  5 = 人生重大节点（职业转型、长期身份变化）
  拿不准时选 3；只有明确的重大节点才给 5，不要乱打分。
- evidence（证据）规则：
  - 必须来自用户明确表达的信息，优先直接抄用户原话片段（可截断），不要总结、不要改写。
  - 无法引用用户原话时留空字符串 ""，绝不允许编造、推断或补全。
  - 反例：用户「Go 怎么做股票系统？」→ evidence 写「用户正在开发股票系统」是推断（错误）；且这条本身也不应作为记忆。
- 示例：「从大厂离职做独立开发」→ domain=career, event_type=change, status=ongoing, importance=4, title=「离职转独立开发」。

【五、profile_facts：稳定画像事实（数组，可为空）】
只记录满足以下之一、且与用户本人相关的信息：
1. 用户明确表达，如「我喜欢…」「我的目标是…」「我长期使用…」「我习惯…」「我是…」
2. 在对话中反复出现且前后一致的事实
兴趣 / 习惯类额外门控（同【一】）：必须满足 a 明确长期 / b 多次提及 / c 影响交互，否则不记录。
不要记录：一次性需求、当前任务上下文、AI 的推测、临时状态。
- 反例：「用户最近研究股票」（临时/上下文）
- 正例：「用户正在学习股票投资，并计划开发选股工具」（有明确目标）
- 反例：「用户喜欢看科幻电影」（顺口一提，无长期/多次/影响交互，不记录）
每条格式：
{"content":"一句关于用户的稳定事实","memory_level":"core|long_term|temporary","confidence":0.9,"evidence":"用户原话片段；无法引用则留空"}
- evidence 规则同【四】：只能来自用户明确表达，禁止为了填字段而总结出不存在的事实。

【六、confidence 规则（禁止推断）】
- confidence 只能基于用户的明确表达：
  - 0.9~1.0：用户直接、明确陈述（如「我是…」「我喜欢…」「我正在…」）
  - 0.7~0.9：同一事实在对话中被用户多次明确陈述，且前后一致
- 严禁通过推断得出信息，以下一律不得写入（无论给多低的分）：
  - 根据用户行为推断身份 / 性格
  - 根据职业领域推测能力
  - 根据提问方向推测兴趣
- 例：用户问「Python 适合量化吗？」→ 不得输出「用户学习量化开发」。
- 只要无法追溯到用户明确表达，就舍弃该条，不要用低 confidence 蒙混写入。

【七、输出】
{
  "is_substantial": true,
  "experiences": [ ... ],
  "profile_facts": [ ... ]
}`,
	},
	PromptKeyMemoryMerge: {
		Remark: "用户画像合并维护 (防画像膨胀漂移)",
		Default: `你是用户画像维护助手。给定【当前画像】和【新增事实】，请合并生成最新的长期用户画像，重点防止画像膨胀与漂移。

【合并规则】
1. 只保留关于用户本人的、长期有效的信息，不要编造不存在的信息。
2. 信息优先级：长期稳定事实 > 当前项目/学习 > 临时状态。
3. 删除：已过期、重复、一次性需求、AI 推测、临时状态的信息。
4. 冲突处理：
   - 新事实明确改变旧状态 → 用新事实更新，例如「以前用 Python，现在主要用 Go」
   - 新事实只是补充 → 合并保留
   - 无法判断 → 保留旧信息，并适当降低 confidence
5. 无法归入固定维度的信息，写进 summary，不要新增维度键。
6. 只合并用户明确表达过的信息；不得从已有维度推导出新事实（例如从职业推断技能、从提问推断兴趣、把 AI 的建议当成用户事实）。无法追溯到用户明确表达的，一律不写。

【dimensions 固定维度】（没有内容则省略该键，不要新增其它键；多值用「、」连接为一个字符串）
- identity：当前身份（职业、角色）；只放当前身份，历史经历放 background
- background：教育、职业经历、行业、资历
- goals：目标
- skills：技能
- projects：项目
- learning_topics：正在学的学习主题
- interests：长期兴趣（未必正在学；与 learning_topics 区分：兴趣是「喜欢」，学习主题是「正在学」）
- preferences：偏好
- habits：习惯
- communication_style：沟通与回答偏好（如喜欢直接、简洁、要例子、不要客套；比 preferences 更具体，直接影响交互）
- language：语言偏好/母语/常用语言
- values：价值观、原则、优先级（长期稳定）
- risk_preference：风险偏好（如稳健/保守/激进）
- decision_style：决策风格（如数据驱动、先试再定；仅当用户明确陈述过才记，不要凭行为推断）
- constraints：约束/限制

【输出】只输出 JSON，不要解释，不要 markdown 代码块：
{"summary":"一段话整体画像，不超过800字","dimensions":{"identity":"","background":"","goals":"","skills":"","projects":"","learning_topics":"","interests":"","preferences":"","habits":"","communication_style":"","language":"","values":"","risk_preference":"","decision_style":"","constraints":""},"tags":["标签"],"confidence":0.8}`,
	},
}

var (
	promptCacheMu sync.RWMutex
	promptCache   map[string]string // key -> system_config 覆盖值 (空串=查过但未配置, 负缓存)
)

// GetPrompt 取提示词: system_config 覆盖值优先, 未配置回落内置默认值。
// 带进程内缓存, 首次访问才查库; 配置页保存后由 RefreshPromptCache 刷新。
func GetPrompt(key string) string {
	def, ok := builtinPrompts[key]
	if !ok {
		return ""
	}
	promptCacheMu.RLock()
	v, hit := promptCache[key]
	promptCacheMu.RUnlock()
	if !hit {
		v = loadPromptOverride(key)
		promptCacheMu.Lock()
		if promptCache == nil {
			promptCache = map[string]string{}
		}
		promptCache[key] = v
		promptCacheMu.Unlock()
	}
	if strings.TrimSpace(v) != "" {
		return v
	}
	return def.Default
}

// loadPromptOverride 查单个提示词的 DB 覆盖值 (无行/出错返回空串 → 用默认值)
func loadPromptOverride(key string) string {
	if DB == nil {
		return ""
	}
	var cfg model.SystemConfig
	if err := DB.Where("`key` = ?", key).First(&cfg).Error; err != nil {
		return ""
	}
	return cfg.Value
}

// RefreshPromptCache 重建提示词缓存 (配置页保存 prompt_* 键后调用)
func RefreshPromptCache() {
	cache := map[string]string{}
	if DB != nil {
		var rows []model.SystemConfig
		if err := DB.Where("`key` LIKE ?", "prompt_%").Find(&rows).Error; err == nil {
			for _, r := range rows {
				cache[r.Key] = r.Value
			}
		}
	}
	promptCacheMu.Lock()
	promptCache = cache
	promptCacheMu.Unlock()
}

// PromptItem 管理接口条目
type PromptItem struct {
	Key          string `json:"key"`
	Remark       string `json:"remark"`
	Value        string `json:"value"` // 当前生效值 (无覆盖时为内置默认)
	DefaultValue string `json:"defaultValue"`
	Overridden   bool   `json:"overridden"` // 是否已被自定义覆盖
}

// ListPrompts 全部内置提示词及当前生效值 (供系统配置页管理)
func ListPrompts() []PromptItem {
	overrides := map[string]string{}
	if DB != nil {
		var rows []model.SystemConfig
		if err := DB.Where("`key` LIKE ?", "prompt_%").Find(&rows).Error; err == nil {
			for _, r := range rows {
				overrides[r.Key] = r.Value
			}
		}
	}
	out := make([]PromptItem, 0, len(promptOrder))
	for _, key := range promptOrder {
		def := builtinPrompts[key]
		v := overrides[key]
		overridden := strings.TrimSpace(v) != ""
		if !overridden {
			v = def.Default
		}
		out = append(out, PromptItem{
			Key: key, Remark: def.Remark, Value: v,
			DefaultValue: def.Default, Overridden: overridden,
		})
	}
	return out
}
