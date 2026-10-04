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
  }
];
