-- +goose Up

-- 词汇量自适应测试 (v2):
-- 1) word_bank: 分级锚定词库 (词的难度由真实分级数据说了算, 不再由 LLM 拍脑袋)。
--    种子 = 每档 40 个典型词 (CEFR/EVP 公开口径人工精选), 结构上支持后续整档扩充
--    (NGSL/NAWL + 开源 CEFR 标注数据导入后 source 标 band-seed 之外的新来源)。
-- 2) vocab_test_items: 考题缓存 (按词全局复用, 与 dictionary_cache 同模式):
--    LLM 只负责给抽样词生成干扰释义, 生成一次永久复用, 题目稳定可排查。

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `word_bank` (
    `id` INT AUTO_INCREMENT PRIMARY KEY,
    `word` VARCHAR(50) NOT NULL,
    `cefr` VARCHAR(4) NOT NULL COMMENT 'A1/A2/B1/B2/C1/C2',
    `source` VARCHAR(30) NOT NULL DEFAULT 'band-seed' COMMENT 'band-seed=内置种子 / 后续扩展来源标识',
    `enabled` TINYINT NOT NULL DEFAULT 1,
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY `uk_word_bank_word` (`word`),
    KEY `idx_word_bank_cefr` (`cefr`, `enabled`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `vocab_test_items` (
    `id` INT AUTO_INCREMENT PRIMARY KEY,
    `word` VARCHAR(50) NOT NULL,
    `cefr` VARCHAR(4) NOT NULL,
    `correct_definition` VARCHAR(255) NOT NULL,
    `distractors` VARCHAR(900) NOT NULL COMMENT '3 个干扰释义 JSON 数组',
    `options_json` VARCHAR(1200) NOT NULL COMMENT '打乱后的 4 选项 JSON 数组',
    `answer` INT NOT NULL COMMENT '正确选项下标 0-3',
    `model` VARCHAR(100) NOT NULL DEFAULT '' COMMENT '生成所用模型 (排查)',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY `uk_vti_word` (`word`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO `word_bank` (`word`, `cefr`, `source`) VALUES
('apple','A1','band-seed'),('water','A1','band-seed'),('house','A1','band-seed'),('dog','A1','band-seed'),('cat','A1','band-seed'),
('book','A1','band-seed'),('run','A1','band-seed'),('eat','A1','band-seed'),('big','A1','band-seed'),('small','A1','band-seed'),
('happy','A1','band-seed'),('mother','A1','band-seed'),('father','A1','band-seed'),('friend','A1','band-seed'),('school','A1','band-seed'),
('milk','A1','band-seed'),('bread','A1','band-seed'),('tree','A1','band-seed'),('sun','A1','band-seed'),('rain','A1','band-seed'),
('hand','A1','band-seed'),('head','A1','band-seed'),('shoe','A1','band-seed'),('door','A1','band-seed'),('window','A1','band-seed'),
('table','A1','band-seed'),('chair','A1','band-seed'),('bed','A1','band-seed'),('sleep','A1','band-seed'),('walk','A1','band-seed'),
('talk','A1','band-seed'),('listen','A1','band-seed'),('red','A1','band-seed'),('blue','A1','band-seed'),('day','A1','band-seed'),
('night','A1','band-seed'),('car','A1','band-seed'),('bird','A1','band-seed'),('fish','A1','band-seed'),('egg','A1','band-seed'),
('weather','A2','band-seed'),('traffic','A2','band-seed'),('invite','A2','band-seed'),('borrow','A2','band-seed'),('spend','A2','band-seed'),
('alone','A2','band-seed'),('already','A2','band-seed'),('birthday','A2','band-seed'),('cheap','A2','band-seed'),('clean','A2','band-seed'),
('country','A2','band-seed'),('dance','A2','band-seed'),('doctor','A2','band-seed'),('dream','A2','band-seed'),('easy','A2','band-seed'),
('empty','A2','band-seed'),('enough','A2','band-seed'),('evening','A2','band-seed'),('exam','A2','band-seed'),('excited','A2','band-seed'),
('famous','A2','band-seed'),('farm','A2','band-seed'),('film','A2','band-seed'),('fresh','A2','band-seed'),('friendly','A2','band-seed'),
('fruit','A2','band-seed'),('game','A2','band-seed'),('garden','A2','band-seed'),('gift','A2','band-seed'),('guess','A2','band-seed'),
('hair','A2','band-seed'),('healthy','A2','band-seed'),('holiday','A2','band-seed'),('hope','A2','band-seed'),('journey','A2','band-seed'),
('kitchen','A2','band-seed'),('laugh','A2','band-seed'),('market','A2','band-seed'),('noisy','A2','band-seed'),('polite','A2','band-seed'),
('achieve','B1','band-seed'),('advice','B1','band-seed'),('afford','B1','band-seed'),('agreement','B1','band-seed'),('ambition','B1','band-seed'),
('annual','B1','band-seed'),('argue','B1','band-seed'),('attempt','B1','band-seed'),('audience','B1','band-seed'),('available','B1','band-seed'),
('aware','B1','band-seed'),('behaviour','B1','band-seed'),('benefit','B1','band-seed'),('budget','B1','band-seed'),('career','B1','band-seed'),
('challenge','B1','band-seed'),('colleague','B1','band-seed'),('community','B1','band-seed'),('compete','B1','band-seed'),('concentrate','B1','band-seed'),
('confident','B1','band-seed'),('consider','B1','band-seed'),('convince','B1','band-seed'),('curious','B1','band-seed'),('describe','B1','band-seed'),
('develop','B1','band-seed'),('economy','B1','band-seed'),('encourage','B1','band-seed'),('environment','B1','band-seed'),('establish','B1','band-seed'),
('evidence','B1','band-seed'),('experience','B1','band-seed'),('explain','B1','band-seed'),('familiar','B1','band-seed'),('improve','B1','band-seed'),
('intend','B1','band-seed'),('knowledge','B1','band-seed'),('manage','B1','band-seed'),('obvious','B1','band-seed'),('opportunity','B1','band-seed'),
('abandon','B2','band-seed'),('absorb','B2','band-seed'),('accurate','B2','band-seed'),('acquire','B2','band-seed'),('adapt','B2','band-seed'),
('adequate','B2','band-seed'),('adjust','B2','band-seed'),('adopt','B2','band-seed'),('alternative','B2','band-seed'),('anticipate','B2','band-seed'),
('appeal','B2','band-seed'),('approach','B2','band-seed'),('appropriate','B2','band-seed'),('artificial','B2','band-seed'),('assemble','B2','band-seed'),
('assess','B2','band-seed'),('assign','B2','band-seed'),('assume','B2','band-seed'),('attribute','B2','band-seed'),('bias','B2','band-seed'),
('campaign','B2','band-seed'),('capable','B2','band-seed'),('cease','B2','band-seed'),('circumstance','B2','band-seed'),('claim','B2','band-seed'),
('commission','B2','band-seed'),('compensate','B2','band-seed'),('component','B2','band-seed'),('compound','B2','band-seed'),('comprise','B2','band-seed'),
('conceal','B2','band-seed'),('conclude','B2','band-seed'),('confine','B2','band-seed'),('conform','B2','band-seed'),('confront','B2','band-seed'),
('consequence','B2','band-seed'),('considerable','B2','band-seed'),('contaminate','B2','band-seed'),('convert','B2','band-seed'),('correspond','B2','band-seed'),
('ambiguous','C1','band-seed'),('coherent','C1','band-seed'),('comprehensive','C1','band-seed'),('contradictory','C1','band-seed'),('crucial','C1','band-seed'),
('dedicate','C1','band-seed'),('dilemma','C1','band-seed'),('diminish','C1','band-seed'),('discrepancy','C1','band-seed'),('dominant','C1','band-seed'),
('elaborate','C1','band-seed'),('eliminate','C1','band-seed'),('empirical','C1','band-seed'),('encompass','C1','band-seed'),('encounter','C1','band-seed'),
('enhance','C1','band-seed'),('evoke','C1','band-seed'),('exhaustive','C1','band-seed'),('feasible','C1','band-seed'),('fluctuate','C1','band-seed'),
('fundamental','C1','band-seed'),('hypothesis','C1','band-seed'),('implement','C1','band-seed'),('implicit','C1','band-seed'),('incentive','C1','band-seed'),
('inevitable','C1','band-seed'),('infer','C1','band-seed'),('inhibit','C1','band-seed'),('insight','C1','band-seed'),('integral','C1','band-seed'),
('intervene','C1','band-seed'),('intricate','C1','band-seed'),('invoke','C1','band-seed'),('manipulate','C1','band-seed'),('mitigate','C1','band-seed'),
('notable','C1','band-seed'),('ominous','C1','band-seed'),('pervasive','C1','band-seed'),('plausible','C1','band-seed'),('precede','C1','band-seed'),
('esoteric','C2','band-seed'),('ubiquitous','C2','band-seed'),('quintessential','C2','band-seed'),('ephemeral','C2','band-seed'),('obfuscate','C2','band-seed'),
('juxtapose','C2','band-seed'),('perfunctory','C2','band-seed'),('laconic','C2','band-seed'),('recalcitrant','C2','band-seed'),('serendipity','C2','band-seed'),
('taciturn','C2','band-seed'),('vernacular','C2','band-seed'),('zealot','C2','band-seed'),('anachronism','C2','band-seed'),('capricious','C2','band-seed'),
('deleterious','C2','band-seed'),('garrulous','C2','band-seed'),('ineffable','C2','band-seed'),('magnanimous','C2','band-seed'),('nepotism','C2','band-seed'),
('oscillate','C2','band-seed'),('parsimonious','C2','band-seed'),('quixotic','C2','band-seed'),('reticent','C2','band-seed'),('temerity','C2','band-seed'),
('umbrage','C2','band-seed'),('vestige','C2','band-seed'),('zenith','C2','band-seed'),('abstruse','C2','band-seed'),('bellicose','C2','band-seed'),
('cacophony','C2','band-seed'),('demagogue','C2','band-seed'),('effervescent','C2','band-seed'),('equivocal','C2','band-seed'),('fastidious','C2','band-seed'),
('pernicious','C2','band-seed'),('repudiate','C2','band-seed'),('transcendent','C2','band-seed'),('vitriolic','C2','band-seed'),('obsequious','C2','band-seed')
ON DUPLICATE KEY UPDATE `word` = `word`;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP TABLE IF EXISTS `vocab_test_items`;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS `word_bank`;
-- +goose StatementEnd
