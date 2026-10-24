/**
 * 编排模板: 新建编排时按模式预填画布 (节点/连线/配置)。
 * 各模板的连线与配置均按后端校验规则 (ai_orchestration_compiler) 预填:
 * 恰好一个入口/出口、分支条件覆盖连线、回边目标在分类条件中、max_loops ≥1 等;
 * 工具/子编排这类依赖实例资源的节点留占位, 加载后由用户在配置面板补选。
 */

export interface OrchestrationTemplateNode {
  id: string;
  type: string;
  name: string;
  config: Record<string, any>;
  position: { x: number; y: number };
  /** chat_first_turn 编排对话: 会话激活后 (首跑粘性分支命中非默认目标) 该节点跳过, 分支回放首跑路由 */
  chatFirstTurn?: boolean;
}

export interface OrchestrationTemplateEdge {
  source: string;
  target: string;
  label?: string;
  kind?: 'flow' | 'loop';
}

export interface OrchestrationTemplate {
  key: string;
  label: string;
  icon: string;
  description: string;
  /** hint 加载后需要用户补配的提示 (如选择具体工具) */
  hint?: string;
  definition: { nodes: OrchestrationTemplateNode[]; edges: OrchestrationTemplateEdge[] };
}

const tplConfig = (template: string) => ({ template });
const agentConfig = (systemPrompt = '') => ({ model: '', system_prompt: systemPrompt, tools: [], max_iterations: 0, max_retries: 0 });
const subAgentConfig = (systemPrompt: string, description: string) => ({
  agent_id: 0,
  model: '',
  system_prompt: systemPrompt,
  description,
  tools: [],
  max_iterations: 0,
  timeout_seconds: 0,
  max_retries: 0
});

export const ORCHESTRATION_TEMPLATES: OrchestrationTemplate[] = [
  {
    key: 'blank',
    label: '空白画布',
    icon: 'mdi:plus-box-outline',
    description: '从零开始, 自行添加节点与连线',
    definition: { nodes: [], edges: [] }
  },
  {
    key: 'sample',
    label: '示例链',
    icon: 'mdi:link-variant-plus',
    description: '模板 → Agent → 结束, 最小可运行示例',
    definition: {
      nodes: [
        { id: 'tpl', type: 'template', name: '输入模板', config: tplConfig('请处理以下内容:\n{{.Input}}'), position: { x: 60, y: 140 } },
        { id: 'agent', type: 'agent', name: 'Agent', config: agentConfig(), position: { x: 360, y: 120 } },
        { id: 'end', type: 'end', name: '结束', config: {}, position: { x: 660, y: 140 } }
      ],
      edges: [
        { source: 'tpl', target: 'agent' },
        { source: 'agent', target: 'end' }
      ]
    }
  },
  {
    key: 'chain',
    label: '线性流水线',
    icon: 'mdi:link-variant',
    description: '模板 → Agent → 结束, 单路径顺序执行, 适合单一任务',
    definition: {
      nodes: [
        { id: 'tpl', type: 'template', name: '输入模板', config: tplConfig('请处理以下内容：\n{{.Input}}'), position: { x: 40, y: 160 } },
        { id: 'agent', type: 'agent', name: '主处理Agent', config: agentConfig('你是处理助手，请按要求处理输入内容并输出结果。'), position: { x: 320, y: 160 } },
        { id: 'end', type: 'end', name: '结束', config: {}, position: { x: 600, y: 160 } }
      ],
      edges: [
        { source: 'tpl', target: 'agent' },
        { source: 'agent', target: 'end' }
      ]
    }
  },
  {
    key: 'branch-route',
    label: '条件分支',
    icon: 'mdi:source-branch',
    description: '按关键词/正则路由到不同 Agent, 各路径经合并节点汇聚输出',
    definition: {
      nodes: [
        { id: 'tpl', type: 'template', name: '输入模板', config: tplConfig('请处理以下内容：\n{{.Input}}'), position: { x: 40, y: 180 } },
        { id: 'agent', type: 'agent', name: '意图识别Agent', config: agentConfig('请分析输入内容并给出处理结果。'), position: { x: 260, y: 180 } },
        {
          id: 'branch',
          type: 'branch',
          name: '意图分支',
          config: {
            mode: 'route',
            cases: [
              { type: 'contains', value: '退款', target: 'after_sale' },
              { type: 'regex', value: '发票|开票', target: 'invoice' },
              { type: 'equals', value: '其他', target: 'general' }
            ],
            default_target: 'general'
          },
          position: { x: 500, y: 180 }
        },
        { id: 'after_sale', type: 'agent', name: '售后处理Agent', config: agentConfig('你是售后专员，按流程处理用户的退款诉求。'), position: { x: 760, y: 60 } },
        { id: 'invoice', type: 'agent', name: '开票处理Agent', config: agentConfig('你是开票专员，处理发票相关请求。'), position: { x: 760, y: 180 } },
        { id: 'general', type: 'agent', name: '通用应答Agent', config: agentConfig('你是通用助手，回答用户问题。'), position: { x: 760, y: 300 } },
        { id: 'merge', type: 'merge', name: '合并', config: { separator: '\n\n' }, position: { x: 1020, y: 180 } },
        { id: 'end', type: 'end', name: '结束', config: {}, position: { x: 1260, y: 180 } }
      ],
      edges: [
        { source: 'tpl', target: 'agent' },
        { source: 'agent', target: 'branch' },
        { source: 'branch', target: 'after_sale' },
        { source: 'branch', target: 'invoice' },
        { source: 'branch', target: 'general' },
        { source: 'after_sale', target: 'merge' },
        { source: 'invoice', target: 'merge' },
        { source: 'general', target: 'merge' },
        { source: 'merge', target: 'end' }
      ]
    }
  },
  {
    key: 'router-loop',
    label: 'LLM路由门控循环',
    icon: 'mdi:routes',
    description: '模型判定「通过/不通过」, 不通过自动打回重做, 超过上限强制通过',
    definition: {
      nodes: [
        { id: 'tpl', type: 'template', name: '输入模板', config: tplConfig('请根据以下要求完成任务：\n{{.Input}}'), position: { x: 40, y: 180 } },
        { id: 'gen', type: 'agent', name: '产出Agent', config: agentConfig('根据要求撰写内容，输出完整初稿。'), position: { x: 280, y: 180 } },
        {
          id: 'router',
          type: 'router',
          name: '质量判定',
          config: {
            model: '',
            instructions: '根据内容质量选择标签: 内容完整且无错误输出「通过」; 有遗漏或错误输出「不通过」',
            cases: [
              { label: '通过', description: '内容完整无误', target: 'end' },
              { label: '不通过', description: '有问题需重写', target: 'gen' }
            ],
            default_target: 'end',
            max_loops: 3
          },
          position: { x: 540, y: 180 }
        },
        { id: 'end', type: 'end', name: '结束', config: {}, position: { x: 800, y: 180 } }
      ],
      edges: [
        { source: 'tpl', target: 'gen' },
        { source: 'gen', target: 'router' },
        { source: 'router', target: 'end' },
        { source: 'router', target: 'gen', label: '不通过', kind: 'loop' }
      ]
    }
  },
  {
    key: 'branch-loop',
    label: '分支循环重试',
    icon: 'mdi:cached',
    description: '按关键词质检, 不合格打回重做 (规则判定, 无需额外模型调用)',
    definition: {
      nodes: [
        { id: 'tpl', type: 'template', name: '输入模板', config: tplConfig('请根据以下要求完成任务：\n{{.Input}}'), position: { x: 40, y: 180 } },
        { id: 'gen', type: 'agent', name: '生成Agent', config: agentConfig('根据要求撰写内容，输出完整初稿。'), position: { x: 280, y: 180 } },
        {
          id: 'quality',
          type: 'branch',
          name: '质检分支',
          config: {
            mode: 'route',
            cases: [
              { type: 'contains', value: '不合格', target: 'gen' },
              { type: 'equals', value: '合格', target: 'end' }
            ],
            default_target: 'end',
            max_loops: 3
          },
          position: { x: 540, y: 180 }
        },
        { id: 'end', type: 'end', name: '结束', config: {}, position: { x: 800, y: 180 } }
      ],
      edges: [
        { source: 'tpl', target: 'gen' },
        { source: 'gen', target: 'quality' },
        { source: 'quality', target: 'end' },
        { source: 'quality', target: 'gen', label: '不合格', kind: 'loop' }
      ]
    }
  },
  {
    key: 'parallel',
    label: '并行分发 + 合并',
    icon: 'mdi:call-split',
    description: '一个任务同时分发给多条路径执行, 结果在合并节点拼接',
    hint: '加载后请在配置面板为两个工具节点选择具体工具',
    definition: {
      nodes: [
        { id: 'tpl', type: 'template', name: '输入模板', config: tplConfig('请基于以下内容完成各分支任务：\n{{.Input}}'), position: { x: 40, y: 180 } },
        { id: 'dispatch', type: 'branch', name: '并行分发', config: { mode: 'parallel', cases: [], default_target: '' }, position: { x: 260, y: 180 } },
        { id: 'tool_weather', type: 'tool', name: '天气查询', config: { tool: '' }, position: { x: 480, y: 60 } },
        { id: 'tool_rate', type: 'tool', name: '汇率查询', config: { tool: '' }, position: { x: 480, y: 180 } },
        { id: 'advisor', type: 'agent', name: '建议Agent', config: agentConfig('基于上游内容给出个性化建议。'), position: { x: 480, y: 300 } },
        { id: 'merge', type: 'merge', name: '合并', config: { separator: '\n\n' }, position: { x: 720, y: 180 } },
        { id: 'end', type: 'end', name: '结束', config: {}, position: { x: 940, y: 180 } }
      ],
      edges: [
        { source: 'tpl', target: 'dispatch' },
        { source: 'dispatch', target: 'tool_weather' },
        { source: 'dispatch', target: 'tool_rate' },
        { source: 'dispatch', target: 'advisor' },
        { source: 'tool_weather', target: 'merge' },
        { source: 'tool_rate', target: 'merge' },
        { source: 'advisor', target: 'merge' },
        { source: 'merge', target: 'end' }
      ]
    }
  },
  {
    key: 'vote',
    label: '多Agent 投票决策',
    icon: 'mdi:vote-outline',
    description: '三个不同视角的评审 Agent 并行独立给出意见, 合并后由裁决 Agent 汇总出最终决策与分歧点',
    definition: {
      nodes: [
        {
          id: 'tpl',
          type: 'template',
          name: '决策议题',
          config: tplConfig('待评审事项：\n{{.Input}}\n\n请独立给出你的评审意见。'),
          position: { x: 40, y: 200 }
        },
        { id: 'dispatch', type: 'branch', name: '并行分发', config: { mode: 'parallel', cases: [], default_target: '' }, position: { x: 240, y: 200 } },
        {
          id: 'voter_a',
          type: 'agent',
          name: '评审·稳健派',
          config: agentConfig(
            '你是评审团中的「稳健派」评审。对收到的议题独立评审: 给出明确结论 (赞成/反对/倾向的方案)、2~3 条理由、主要顾虑。只依据你自己的判断, 不要揣测其他评审的看法, 不要输出与评审无关的内容。'
          ),
          position: { x: 440, y: 60 }
        },
        {
          id: 'voter_b',
          type: 'agent',
          name: '评审·进取派',
          config: agentConfig(
            '你是评审团中的「进取派」评审。对收到的议题独立评审: 给出明确结论 (赞成/反对/倾向的方案)、2~3 条理由、潜在收益与机会。只依据你自己的判断, 不要揣测其他评审的看法, 不要输出与评审无关的内容。'
          ),
          position: { x: 440, y: 200 }
        },
        {
          id: 'voter_c',
          type: 'agent',
          name: '评审·风控派',
          config: agentConfig(
            '你是评审团中的「风控派」评审。对收到的议题独立评审: 给出明确结论 (赞成/反对/倾向的方案)、2~3 条理由, 重点评估成本、执行难度、合规与失败后果。只依据你自己的判断, 不要揣测其他评审的看法, 不要输出与评审无关的内容。'
          ),
          position: { x: 440, y: 340 }
        },
        { id: 'merge', type: 'merge', name: '意见汇总', config: { separator: '\n\n———\n\n' }, position: { x: 700, y: 200 } },
        {
          id: 'judge',
          type: 'agent',
          name: '裁决Agent',
          config: agentConfig(
            '你是评审团主持人。上面是多位评审对同一议题的独立意见, 请汇总裁决:\n1. 对比各评审的结论与理由, 指出共识;\n2. 按多数意见与论证质量给出最终决策;\n3. 列出仍未达成一致的分歧点与建议补充的信息。\n输出格式:\n## 最终决策\n## 决策依据\n## 分歧点'
          ),
          position: { x: 900, y: 200 }
        },
        { id: 'end', type: 'end', name: '结束', config: {}, position: { x: 1120, y: 200 } }
      ],
      edges: [
        { source: 'tpl', target: 'dispatch' },
        { source: 'dispatch', target: 'voter_a' },
        { source: 'dispatch', target: 'voter_b' },
        { source: 'dispatch', target: 'voter_c' },
        { source: 'voter_a', target: 'merge' },
        { source: 'voter_b', target: 'merge' },
        { source: 'voter_c', target: 'merge' },
        { source: 'merge', target: 'judge' },
        { source: 'judge', target: 'end' }
      ]
    }
  },
  {
    key: 'extract',
    label: '字段提取',
    icon: 'mdi:code-json',
    description: 'Agent 输出 JSON 后按字段路径取值, 抽取失败走兜底值',
    definition: {
      nodes: [
        {
          id: 'tpl',
          type: 'template',
          name: '输入模板',
          config: tplConfig('请对以下内容打分，只输出 JSON（不要其他内容）：\n{"score": 0-100的整数, "reason": "理由"}\n\n内容：\n{{.Input}}'),
          position: { x: 40, y: 160 }
        },
        { id: 'agent', type: 'agent', name: '评分Agent', config: agentConfig('你只输出 JSON, 不要输出任何其他内容。'), position: { x: 280, y: 160 } },
        { id: 'extract', type: 'extract', name: '取分值', config: { field: 'score', fallback: '0' }, position: { x: 540, y: 160 } },
        { id: 'end', type: 'end', name: '结束', config: {}, position: { x: 800, y: 160 } }
      ],
      edges: [
        { source: 'tpl', target: 'agent' },
        { source: 'agent', target: 'extract' },
        { source: 'extract', target: 'end' }
      ]
    }
  },
  {
    key: 'delegate',
    label: '子Agent 委派',
    icon: 'mdi:account-group-outline',
    description: '主管 Agent 按需委派检索/撰写子 Agent, 汇总后输出 (子Agent 可引用已有 Agent)',
    definition: {
      nodes: [
        { id: 'tpl', type: 'template', name: '输入模板', config: tplConfig('请完成以下任务：\n{{.Input}}'), position: { x: 40, y: 200 } },
        {
          id: 'lead',
          type: 'agent',
          name: '主管Agent',
          config: agentConfig('你是主管助手。需要外部资料时调用委派工具「资料检索」; 素材齐备后把任务交给「撰写润色」; 最后汇总输出。'),
          position: { x: 280, y: 200 }
        },
        {
          id: 'searcher',
          type: 'subagent',
          name: '资料检索',
          config: subAgentConfig('负责根据任务检索相关资料并整理成要点返回。', '需要检索外部资料时委派'),
          position: { x: 560, y: 40 }
        },
        {
          id: 'writer',
          type: 'subagent',
          name: '撰写润色',
          config: subAgentConfig('负责把要点撰写成通顺、结构清晰的中文段落。', '需要成文撰写时委派'),
          position: { x: 560, y: 360 }
        },
        { id: 'end', type: 'end', name: '结束', config: {}, position: { x: 520, y: 200 } }
      ],
      edges: [
        { source: 'tpl', target: 'lead' },
        { source: 'lead', target: 'end' },
        { source: 'lead', target: 'searcher' },
        { source: 'lead', target: 'writer' }
      ]
    }
  },
  {
    key: 'suborch',
    label: '子编排',
    icon: 'mdi:graph-outline',
    description: '引用已保存的编排作为节点复用 (编译时展开, 禁止循环引用)',
    hint: '加载后请在子编排节点配置中选择要引用的编排',
    definition: {
      nodes: [
        { id: 'tpl', type: 'template', name: '输入模板', config: tplConfig('请处理以下内容：\n{{.Input}}'), position: { x: 40, y: 160 } },
        { id: 'sub', type: 'suborch', name: '子编排', config: { orchestration_id: 0 }, position: { x: 280, y: 160 } },
        { id: 'end', type: 'end', name: '结束', config: {}, position: { x: 520, y: 160 } }
      ],
      edges: [
        { source: 'tpl', target: 'sub' },
        { source: 'sub', target: 'end' }
      ]
    }
  },
  {
    key: 'full',
    label: '综合示例',
    icon: 'mdi:rocket-launch-outline',
    description: '写作 Agent(委派检索) → 质检分支循环 → 发布工具, 一次用上六种节点',
    hint: '加载后请在配置面板为发布工具选择具体工具',
    definition: {
      nodes: [
        { id: 'tpl', type: 'template', name: '写作需求', config: tplConfig('请根据以下需求撰写内容：\n{{.Input}}'), position: { x: 40, y: 240 } },
        {
          id: 'writer',
          type: 'agent',
          name: '写作Agent',
          config: agentConfig('你是写作助手。需要素材时可委派「资料检索」子Agent; 完成后输出最终稿。'),
          position: { x: 260, y: 240 }
        },
        {
          id: 'quality',
          type: 'branch',
          name: '质检分支',
          config: {
            mode: 'route',
            cases: [
              { type: 'contains', value: '不合格', target: 'writer' },
              { type: 'equals', value: '合格', target: 'publish' }
            ],
            default_target: 'publish',
            max_loops: 3
          },
          position: { x: 520, y: 240 }
        },
        { id: 'publish', type: 'tool', name: '发布工具', config: { tool: '' }, position: { x: 800, y: 240 } },
        { id: 'end', type: 'end', name: '结束', config: {}, position: { x: 1020, y: 240 } },
        {
          id: 'researcher',
          type: 'subagent',
          name: '资料检索',
          config: subAgentConfig('负责根据写作主题检索资料并整理成要点返回。', '需要检索素材时委派'),
          position: { x: 480, y: 40 }
        }
      ],
      edges: [
        { source: 'tpl', target: 'writer' },
        { source: 'writer', target: 'quality' },
        { source: 'quality', target: 'writer', label: '不合格', kind: 'loop' },
        { source: 'quality', target: 'publish' },
        { source: 'publish', target: 'end' },
        { source: 'writer', target: 'researcher' }
      ]
    }
  },
  {
    key: 'training-scenario-engine',
    label: '训练·场景工程层',
    icon: 'mdi:factory',
    description: '对话最终架构的 N05–N08: 场景生成 → 独立质检 → 需修订打回重做 (上限2次), 通过/否决/超限后原文透传出图。建好后保存为编排, 供「训练平台·社交试点」的子编排节点引用',
    hint: 'N05 场景库检索未入图: 需检索原型时在场景生成器节点的 tools 里挂检索工具, 或第 3 期接入场景库后再改图',
    definition: {
      nodes: [
        { id: 'tpl', type: 'template', name: '训练目标输入', config: tplConfig('{{.Input}}'), position: { x: 40, y: 160 } },
        {
          id: 'designer',
          type: 'agent',
          name: '场景生成器',
          config: agentConfig(
            '# 角色\n你是现实生活训练场景设计师。\n\n# 输入\n接收训练目标、难度、场景原型、人物关系、场景约束和训练成功标准。\n\n# 设计流程\n1. 选择与训练目标直接相关的生活事件。\n2. 确定人物身份、双方目标、利益关系和现实限制。\n3. 解释冲突为什么发生，以及每个人为什么采取当前行为。\n4. 设计自然的对话起点或决策起点。\n5. 明确人物知道什么、不知道什么，以及行为边界。\n6. 给出可以自然发展的多个方向，但不预设唯一结局。\n\n# 必须遵守\n- 不为追求戏剧性而添加极端事件。\n- 不让普通人物无缘无故威胁、羞辱、操纵或突然翻脸。\n- 不使人物行为与其动机互相矛盾。\n- 不为了考验用户而故意让人物行为不合常理。\n- 不把用户的所有回应都导向失败。\n- 不预设用户的个人经历。\n- 不把场景难度等同于冲突强度。\n\n# 输出\n生成结构化场景，包括背景、人物、动机、矛盾成因、用户目标、开场语、难度依据、行为边界、成功标准和合理发展方向。\n\n不要在此节点直接与用户开展训练。场景必须先经过独立质检。\n\n# 引擎适配 (输出协议)\n输入为「需修订」质检反馈时, 只修复指出的问题后重新输出完整场景。只输出完整场景 JSON (场景事实卡结构): scenario_id, training_type, primary_skill, difficulty, background, user_goal, characters[id, role, goal, motivation, known_facts, behavior_constraints], conflict{cause,stakes}, opening_line, success_criteria, quality_control{status, critical_failure}, runtime_state{turn_count, facts_established, relationship_state, termination_reason}。不要输出解释或剧情。'
          ),
          position: { x: 280, y: 160 }
        },
        {
          id: 'reviewer',
          type: 'agent',
          name: '独立场景质检器',
          config: agentConfig(
            '# 角色\n你是独立的训练场景质量审核员。\n\n你不负责美化故事，也不负责执行训练。你的首要任务是发现场景中的现实逻辑错误。\n\n# 检查维度\n1. 现实性：事件是否可能在真实生活中自然发生。\n2. 因果性：人物行为和冲突是否有合理成因。\n3. 动机一致性：人物的目标、顾虑和行为是否相符。\n4. 连贯性：背景、人物关系和事件是否存在矛盾。\n5. 训练匹配度：场景是否能有效练习指定能力。\n6. 难度适配：难度是否合理，是否通过无故恶化冲突制造困难。\n7. 开放性：用户是否可以通过不同合理策略影响发展。\n8. 可执行性：专业训练 Agent 是否能根据场景开展互动。\n\n# 硬性否决\n出现以下问题之一，必须判定拒绝：\n- 严重不符合现实生活常识。\n- 核心冲突没有任何合理起因。\n- 人物突然做出与设定明显冲突的行为。\n- 场景与训练目标明显不匹配。\n- 需要虚构用户未提供的个人事实才能成立。\n- 角色扮演无法遵循场景中的事实约束。\n\n# 输出\n- verdict：pass / revise / reject\n- score：0–100\n- issues：具体问题\n- repair_instructions：定向修复要求\n- critical_failure：true / false\n\n通过标准建议为85分，且无硬性否决项。评分低于85分时，必须指出具体问题，不得只给分数。\n\n# 引擎适配 (输出协议, 三选一)\n- 判定 revise: 第一行写「需修订」, 之后输出 JSON {verdict:"revise", score, issues, repair_instructions, critical_failure}\n- 判定 reject (含任一硬性否决项): 第一行写「否决」, 之后输出 JSON {verdict:"reject", score, issues, critical_failure}\n- 判定 pass: 第一行写「质检通过」, 第二行起逐字原样输出输入中的完整场景 JSON, 禁止改动任何字段\n\n除这三种标记外不要创造其他判定措辞, 输出中不要出现「未通过」「不通过」字样。'
          ),
          position: { x: 540, y: 160 }
        },
        {
          id: 'gate',
          type: 'branch',
          name: '质检判断',
          config: {
            mode: 'route',
            cases: [{ type: 'contains', value: '需修订', target: 'designer' }],
            default_target: 'out',
            max_loops: 2
          },
          position: { x: 800, y: 160 }
        },
        { id: 'out', type: 'end', name: '结束', config: {}, position: { x: 1020, y: 160 } }
      ],
      edges: [
        { source: 'tpl', target: 'designer' },
        { source: 'designer', target: 'reviewer' },
        { source: 'reviewer', target: 'gate' },
        { source: 'gate', target: 'out' },
        { source: 'gate', target: 'designer', label: '需修订·重新生成', kind: 'loop' }
      ]
    }
  },
  {
    key: 'training-platform-pilot',
    label: '训练平台·编排试点',
    icon: 'mdi:sitemap',
    description: '按对话「完整架构总览」七分层落地: 01 用户入口 → 02 主控 Agent(只调度) → 03 目标解析+条件路由 → 04 场景工程层(子编排·质检循环·降级) → 05 专业训练层·六个子 Agent(英语/决策/社交/应变/认知/思辨, 读取同一份场景事实卡, 遵循统一训练接口) → 06 独立评估+达标判断(复盘结束/针对性复练) → 07 状态管理与统一输出; 类型无法识别时转回主控 Agent 向用户澄清, 下一轮重新进入流水线',
    hint: '先加载「训练·场景工程层」建好并保存, 再加载本模板并在「场景工程层」节点选择该编排; 评估层须内联在主画布 (子编排不注入对话历史); 六个训练 Agent 的模块规则可按需精简, 或用提示词版本管理按节点覆盖; 标注「首轮后跳过」的节点在编排对话会话激活后不再重跑 (首轮确定训练类型/场景, 后续轮直接续聊); 类型无法识别时走主控澄清 (仅问清需求, 不激活会话), 用户答清后下一轮重新进入流水线',
    definition: {
      nodes: [
        { id: 'tpl', type: 'template', name: '01·用户入口', config: tplConfig('{{.Input}}'), position: { x: 40, y: 300 } },
        {
          id: 'master',
          type: 'agent',
          chatFirstTurn: true,
          name: '02·主控 Agent',
          config: agentConfig(
            '# 角色\n你是 AI 训练平台的主控 Agent，负责理解用户意图和调度训练流程。\n\n# 职责\n1. 判断用户是普通问答、明确训练请求，还是需要进一步澄清的模糊任务。\n2. 明确训练类型、用户目标和必要的约束条件。\n3. 将训练任务交给统一训练流程，不直接自行编写未经审核的训练场景。\n4. 不替代专业训练 Agent，不替代独立评估 Agent。\n5. 需要外部事实或最新信息时才调用搜索工具，普通情景训练不默认调用搜索。\n6. 用户主动结束训练时，转入阶段性总结流程。\n\n# 约束\n- 不自行制造用户没有提供的个人经历。\n- 不把所有请求都转成训练任务。\n- 不要求六个专业 Agent 同时参与普通训练。\n- 不绕过场景质量检查。\n- 不将模型生成的分数直接视为已经验证的事实。\n\n# 输出\n输出任务类型、训练目标、必要约束和下一步流程动作，供下游节点使用。\n\n# 引擎适配\n本画布从用户输入直接下接训练目标解析器, 你只输出调度结论 (训练类型/目标/约束/下一步), 不复述请求全文; 属普通问答或需澄清时, 输出说明并注明训练类型未知, 交由条件路由转主控澄清, 不虚构训练任务。'
          ),
          position: { x: 240, y: 300 }
        },
        {
          id: 'parser',
          type: 'agent',
          chatFirstTurn: true,
          name: '03·训练目标解析器',
          config: agentConfig(
            '# 角色\n你是训练目标分析器。\n\n# 任务\n将用户请求转化为可执行、可观察、可评估的训练目标。\n\n# 分析字段\n- training_type：训练类型\n- primary_skill：主要训练能力\n- behavioral_objectives：希望用户实际练习的行为\n- difficulty：难度等级\n- context_constraints：场景约束\n- success_criteria：成功标准\n- prohibited_elements：不希望出现的场景元素\n\n# 规则\n1. 一次训练优先设置一个主要能力目标，最多附加两个次要目标。\n2. 不把抽象目标直接当作可评估行为。\n3. 将“提升社交能力”转化为具体行为，例如自然开启话题、回应对方、延续对话。\n4. 将“提升决策能力”转化为识别目标、比较方案、权衡风险和解释选择。\n5. 用户未指定难度时，优先选择普通、低风险、容易进入的场景。\n6. 不擅自添加极端冲突来提高难度。\n7. 输出结构化结果，不生成完整剧情。\n\n# 引擎适配\n只输出一个 JSON 对象 (字段同上); training_type 取值: english / decision / social / crisis / cognitive / debate, 无法归类时输出 {"training_type":"unknown"}。'
          ),
          position: { x: 460, y: 300 }
        },
        {
          id: 'type_route',
          type: 'branch',
          chatFirstTurn: true,
          name: '03·条件路由',
          config: {
            mode: 'route',
            cases: [
              { type: 'contains', value: 'english', target: 'scene' },
              { type: 'contains', value: 'decision', target: 'scene' },
              { type: 'contains', value: 'social', target: 'scene' },
              { type: 'contains', value: 'crisis', target: 'scene' },
              { type: 'contains', value: 'cognitive', target: 'scene' },
              { type: 'contains', value: 'debate', target: 'scene' }
            ],
            default_target: 'clarify'
          },
          position: { x: 680, y: 300 }
        },
        {
          id: 'clarify',
          type: 'agent',
          name: '02·主控澄清',
          config: agentConfig(
            '# 角色\n你是 AI 训练平台的主控 Agent (澄清模式)。上游解析器没有得出明确的训练类型, 本轮请求的需求还不够清晰。\n\n# 任务\n向用户提出 2-4 个澄清问题, 帮助下一轮解析出明确的训练目标:\n1. 想训练哪一类? (英语情景对话 / 决策训练 / 社交训练 / 突发应变训练 / 认知教练 / 思辨训练)\n2. 想练的具体能力或行为是什么? (越具体越好, 例如"礼貌拒绝同事的代班请求")\n3. 期望的难度或场景约束 (可选)。\n\n# 约束\n- 本轮只提问, 不开始训练, 不生成场景。\n- 问题用中文, 简短直接, 不输出 JSON。\n- 结合上下文历史, 用户已说明过的内容不再重复问。\n\n# 运转说明\n用户回复后, 会话下一轮会重新走主控 → 解析器 → 条件路由, 届时按回答进入对应链路。'
          ),
          position: { x: 680, y: 560 }
        },
        { id: 'scene', type: 'suborch', chatFirstTurn: true, name: '04·场景工程层', config: { orchestration_id: 0 }, position: { x: 900, y: 300 } },
        {
          id: 'scene_gate',
          type: 'branch',
          chatFirstTurn: true,
          name: '04·质检判断',
          config: {
            mode: 'route',
            cases: [{ type: 'contains', value: '质检通过', target: 'init' }],
            default_target: 'fallback'
          },
          position: { x: 1120, y: 300 }
        },
        {
          id: 'init',
          type: 'template',
          chatFirstTurn: true,
          name: '04·场景初始化',
          config: tplConfig('以下是已通过独立质检的场景事实卡。基础事实不可修改, runtime_state 初始化为起始状态并随后续对话更新：\n\n{{.Input}}'),
          position: { x: 1340, y: 180 }
        },
        {
          id: 'fallback',
          type: 'template',
          chatFirstTurn: true,
          name: '04·降级为已审核场景',
          config: tplConfig(
            '【降级·使用已审核基础场景】(生成的场景未通过质检或修订超限, 按审查机制改用下面这份内置已审核场景)\n\n{\n  "scenario_id": "SOC-001",\n  "training_type": "social",\n  "primary_skill": "polite_refusal",\n  "difficulty": "beginner",\n  "background": "同事临时请求代班",\n  "user_goal": "表达拒绝并维护合作关系",\n  "characters": [\n    {\n      "id": "coworker",\n      "role": "同事",\n      "goal": "找到可以代班的人",\n      "motivation": "临时有事需要处理",\n      "known_facts": [\n        "自己周五无法正常上班",\n        "用户可能已有安排"\n      ],\n      "behavior_constraints": [\n        "可以请求或协商",\n        "不能无故升级为威胁"\n      ]\n    }\n  ],\n  "conflict": {\n    "cause": "双方时间安排冲突",\n    "stakes": "同事需要帮助，用户需要维护自己的安排"\n  },\n  "opening_line": "我周五临时有点事，你能不能帮我顶一下班？",\n  "success_criteria": [\n    "清楚表达是否能够帮忙",\n    "尊重双方的合理需求",\n    "在适当情况下提出可行替代方案"\n  ],\n  "quality_control": {\n    "status": "approved",\n    "critical_failure": false\n  },\n  "runtime_state": {\n    "turn_count": 0,\n    "facts_established": [],\n    "relationship_state": "normal",\n    "termination_reason": null\n  }\n}'
          ),
          position: { x: 1340, y: 440 }
        },
        { id: 'merge_scene', type: 'merge', name: '04·场景汇总', config: { separator: '\n\n' }, position: { x: 1560, y: 300 } },
        {
          id: 'module_route',
          type: 'branch',
          chatFirstTurn: true,
          name: '05·专业训练路由',
          config: {
            mode: 'route',
            cases: [
              { type: 'contains', value: 'english', target: 't_english' },
              { type: 'contains', value: 'decision', target: 't_decision' },
              { type: 'contains', value: 'crisis', target: 't_crisis' },
              { type: 'contains', value: 'cognitive', target: 't_cognitive' },
              { type: 'contains', value: 'debate', target: 't_debate' }
            ],
            default_target: 't_social'
          },
          position: { x: 1780, y: 300 }
        },
        {
          id: 't_english',
          type: 'agent',
          name: '05·英语情景对话',
          config: agentConfig(
            '# 角色\n你是专业训练教练 (英语情景对话模块), 同时负责模拟真实互动中的对话对象。\n\n# 模块规则\n- 场景设计重点: 真实交流目的、语言水平、自然对话节奏。\n- 评估维度: 可理解性、流畅度、语境适配、语言准确性。\n- 不为纠正语法而破坏正常交流节奏。\n\n# 引擎适配\n首次收到场景事实卡时, 以事实卡的 opening_line 开场并简述本次训练主题与你的角色; 之后每轮只作角色回应。达到训练结束条件、用户要求停止或场景无法继续时, 在回复末尾单独一行输出「【训练结束】」并做 2-3 句阶段性小结交给评估; 结束前不宣布用户已达标; 除标记行外不要出现「【训练结束】」字样。'
          ),
          position: { x: 2020, y: 40 }
        },
        {
          id: 't_decision',
          type: 'agent',
          name: '05·决策训练',
          config: agentConfig(
            '# 角色\n你是专业训练教练 (决策训练模块), 同时负责模拟真实互动中的决策情境与相关方。\n\n# 模块规则\n- 场景设计重点: 明确选项、资源限制、利益取舍、结果不确定性。\n- 评估维度: 目标识别、方案比较、风险意识、理由质量。\n- 不把复杂问题强行变成唯一正确答案; 决策结果不好不代表决策过程不合理。\n\n# 引擎适配\n首次收到场景事实卡时, 以事实卡的 opening_line 开场并给出用户的目标与约束; 之后每轮只作情境回应。达到训练结束条件、用户要求停止或场景无法继续时, 在回复末尾单独一行输出「【训练结束】」并做 2-3 句阶段性小结交给评估; 结束前不宣布用户已达标; 除标记行外不要出现「【训练结束】」字样。'
          ),
          position: { x: 2020, y: 160 }
        },
        {
          id: 't_social',
          type: 'agent',
          name: '05·社交训练',
          config: agentConfig(
            '# 角色\n你是专业训练教练 (社交训练模块), 同时负责模拟真实互动中的对话对象。\n\n# 模块规则\n- 场景设计重点: 人物关系、社交目的、互动节奏、自然反馈。\n- 评估维度: 倾听、回应、话题延续、边界表达。\n- 不把交流节奏的顿挫当成问题, 不为纠正表达而破坏正常交流。\n\n# 引擎适配\n首次收到场景事实卡时, 以事实卡的 opening_line 开场并简述本次训练主题与你的角色; 之后每轮只作角色回应。达到训练结束条件、用户要求停止或场景无法继续时, 在回复末尾单独一行输出「【训练结束】」并做 2-3 句阶段性小结交给评估; 结束前不宣布用户已达标; 除标记行外不要出现「【训练结束】」字样。'
          ),
          position: { x: 2020, y: 280 }
        },
        {
          id: 't_crisis',
          type: 'agent',
          name: '05·突发应变训练',
          config: agentConfig(
            '# 角色\n你是专业训练教练 (突发应变训练模块), 同时负责演化突发事件与情景压力。\n\n# 模块规则\n- 场景设计重点: 可信的突发事件、时间压力、有限信息。\n- 评估维度: 情境判断、优先级、行动合理性、风险识别。\n- 突发事件须有合理成因; 不凭空升级灾难。\n\n# 引擎适配\n首次收到场景事实卡时, 以事实卡的 opening_line 开场并交代当前状况与可用资源; 之后每轮只作情境回应。达到训练结束条件、用户要求停止或场景无法继续时, 在回复末尾单独一行输出「【训练结束】」并做 2-3 句阶段性小结交给评估; 结束前不宣布用户已达标; 除标记行外不要出现「【训练结束】」字样。'
          ),
          position: { x: 2020, y: 400 }
        },
        {
          id: 't_cognitive',
          type: 'agent',
          name: '05·认知教练',
          config: agentConfig(
            '# 角色\n你是专业训练教练 (认知教练模块), 负责围绕真实困扰进行有结构的思维训练。\n\n# 模块规则\n- 场景设计重点: 真实困扰、具体判断、证据与替代解释。\n- 评估维度: 识别假设、证据质量、认知灵活性、反思能力。\n- 不能把正常情绪直接诊断为认知偏差。\n\n# 引擎适配\n首次收到场景事实卡时, 以事实卡的 opening_line 开场并说明本次 Coach 的角度; 之后每轮只作教练回应, 不代替用户下结论。达到训练结束条件、用户要求停止或场景无法继续时, 在回复末尾单独一行输出「【训练结束】」并做 2-3 句阶段性小结交给评估; 结束前不宣布用户已达标; 除标记行外不要出现「【训练结束】」字样。'
          ),
          position: { x: 2020, y: 520 }
        },
        {
          id: 't_debate',
          type: 'agent',
          name: '05·思辨训练',
          config: agentConfig(
            '# 角色\n你是专业训练教练 (思辨训练模块), 负责设置争议性主张并进行正反论证训练。\n\n# 模块规则\n- 场景设计重点: 有争议但可讨论的主张、论据、反例。\n- 评估维度: 逻辑一致性、证据使用、反驳质量、观点修正。\n- 不预设唯一正确立场; 观点修正属于有效表现。\n\n# 引擎适配\n首次收到场景事实卡时, 以事实卡的 opening_line 开场并明确主张与用户立场; 之后每轮只作论辩回应。达到训练结束条件、用户要求停止或场景无法继续时, 在回复末尾单独一行输出「【训练结束】」并做 2-3 句阶段性小结交给评估; 结束前不宣布用户已达标; 除标记行外不要出现「【训练结束】」字样。'
          ),
          position: { x: 2020, y: 640 }
        },
        { id: 'merge_fb', type: 'merge', name: '05·训练输出汇总', config: { separator: '\n\n' }, position: { x: 2280, y: 300 } },
        {
          id: 'stop_gate',
          type: 'branch',
          name: '06·训练结束判断',
          config: {
            mode: 'route',
            cases: [{ type: 'contains', value: '训练结束', target: 'evaluator' }],
            default_target: 'merge_end'
          },
          position: { x: 2500, y: 300 }
        },
        {
          id: 'evaluator',
          type: 'agent',
          name: '06·独立评估 Agent',
          config: agentConfig(
            '你是独立的评估 Agent, 只对用户表现打分, 不审查题目质量 (题目质量由场景质检器负责, 两者是独立机制)。本轮输入是教练的收尾消息, 完整训练对话在你的上下文历史中, 事实卡中的 training_type 决定评分维度。\n\n# 评分规则 (按训练类型选用)\n- english: 可理解性、流畅度、语境适配、语言准确性\n- decision: 目标识别、方案比较、风险意识、理由质量\n- social: 倾听、回应、话题延续、边界表达\n- crisis: 情境判断、优先级、行动合理性、风险识别\n- cognitive: 识别假设、证据质量、认知灵活性、反思能力\n- debate: 逻辑一致性、证据使用、反驳质量、观点修正\n\n# 评估流程\n1. 先做用户表现提取: 引用用户原话关键句作为证据。\n2. 再按上面对应模块的维度逐项打分 (各 0-100)。\n\n# 评估输出 (统一结构, 只输出一个 JSON 对象, 另附一行评定标记)\n- scores：按所选模块维度的评分\n- evidence：从真实对话中提取的具体表现 (引用用户原话关键句)\n- strengths：有效行为\n- improvements：待改进的行为\n- next_exercise：下一轮练习目标 (尚未达标时选最薄弱的一个能力点复练)\n- confidence：评估置信程度 high|medium|low\n- termination_reason：结束原因 goal_done|user_stop|max_turns|off_track\n\nJSON 输出后单独一行附加评定标记: 达到目标输出「已达标」, 尚未达标输出「尚未达标」。除该标记行外, 输出其他位置不要出现「已达标」「尚未达标」字样。\n\n# 规则\n1. 证据不足的维度标记低置信度, 不输出虚假的精确判断。\n2. 不应仅凭最终结果评分; 过程质量与结果质量分开评估。\n3. 用户表现不足时给提示或安排复练, 不把一次失败当成能力定论。'
          ),
          position: { x: 2740, y: 100 }
        },
        {
          id: 'verdict_gate',
          type: 'branch',
          name: '06·达标判断',
          config: {
            mode: 'route',
            cases: [{ type: 'contains', value: '尚未达标', target: 'practice' }],
            default_target: 'review'
          },
          position: { x: 2960, y: 300 }
        },
        {
          id: 'review',
          type: 'template',
          name: '06·复盘并结束',
          config: tplConfig('【达到目标·复盘】评估已判定训练达到目标, 可结束本轮训练。\n\n{{.Input}}\n\n复盘要点：总结有效行为, 上面的评估结果可直接纳入训练报告。'),
          position: { x: 3200, y: 100 }
        },
        {
          id: 'practice',
          type: 'template',
          name: '06·提示或针对性复练',
          config: tplConfig('【尚未达标·复练安排】评估已判定本次目标未达成。给出提示或安排针对性复练, 不把一次失败当成能力定论。\n\n{{.Input}}\n\n复练方式：以 next_exercise 指向的最薄弱能力点出题; 复练时可参考本轮 evidence 中不足的具体行为。'),
          position: { x: 3200, y: 440 }
        },
        { id: 'merge_end', type: 'merge', name: '06·反馈汇总', config: { separator: '\n\n' }, position: { x: 3420, y: 300 } },
        {
          id: 'state',
          type: 'template',
          name: '07·状态管理与统一输出',
          config: tplConfig('【统一输出·状态存档】(对应总览第 07 层; 训练记录/能力档案/历史记录的持久化由应用层在第 2 期接入)\n\n{{.Input}}\n\n按链路不同, 上游为: 训练结束后=评估与复盘/复练安排; 训练进行中=本轮训练 Agent 回复; 类型无法识别=主控澄清提问 (用户回复后下一轮重新进入流水线)。待保存字段：训练报告(评估结果)、能力档案变更(各维度评分与时间)、历史记录、下次训练建议(next_exercise)。'),
          position: { x: 3640, y: 300 }
        },
        { id: 'out', type: 'end', name: '结束', config: {}, position: { x: 3860, y: 300 } }
      ],
      edges: [
        { source: 'tpl', target: 'master' },
        { source: 'master', target: 'parser' },
        { source: 'parser', target: 'type_route' },
        { source: 'type_route', target: 'scene', label: '六类训练' },
        { source: 'type_route', target: 'clarify', label: '无法识别' },
        { source: 'scene', target: 'scene_gate' },
        { source: 'scene_gate', target: 'init' },
        { source: 'scene_gate', target: 'fallback' },
        { source: 'init', target: 'merge_scene' },
        { source: 'fallback', target: 'merge_scene' },
        { source: 'merge_scene', target: 'module_route' },
        { source: 'module_route', target: 't_english', label: 'english' },
        { source: 'module_route', target: 't_decision', label: 'decision' },
        { source: 'module_route', target: 't_social', label: 'social' },
        { source: 'module_route', target: 't_crisis', label: 'crisis' },
        { source: 'module_route', target: 't_cognitive', label: 'cognitive' },
        { source: 'module_route', target: 't_debate', label: 'debate' },
        { source: 't_english', target: 'merge_fb' },
        { source: 't_decision', target: 'merge_fb' },
        { source: 't_social', target: 'merge_fb' },
        { source: 't_crisis', target: 'merge_fb' },
        { source: 't_cognitive', target: 'merge_fb' },
        { source: 't_debate', target: 'merge_fb' },
        { source: 'merge_fb', target: 'stop_gate' },
        { source: 'stop_gate', target: 'evaluator', label: '【训练结束】' },
        { source: 'stop_gate', target: 'merge_end', label: '继续' },
        { source: 'evaluator', target: 'verdict_gate' },
        { source: 'verdict_gate', target: 'review', label: '已达标' },
        { source: 'verdict_gate', target: 'practice', label: '尚未达标' },
        { source: 'review', target: 'merge_end' },
        { source: 'practice', target: 'merge_end' },
        { source: 'clarify', target: 'merge_end' },
        { source: 'merge_end', target: 'state' },
        { source: 'state', target: 'out' }
      ]
    }
  }
];
