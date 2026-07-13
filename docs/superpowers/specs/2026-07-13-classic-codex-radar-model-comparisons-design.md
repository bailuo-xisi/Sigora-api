# Classic Codex Radar 模型等级展示设计

## 目标

在 `web/classic` 的 Codex Radar 页面中展示 CodexRadar 公共摘要里的全部模型/推理等级组合，而不是只展示 `model_iq.latest` 的单个组合。页面必须随 `current.json` 的 `comparisons` 数据自动更新。

## 数据与状态

- 数据源继续使用 `https://codexradar.com/current.json`，不新增后端接口。
- 从 `model_iq.latest` 构造主组合，并从 `model_iq.comparisons` 读取其余组合。
- 主组合的 key 为稳定的 `primary`；比较组合使用原始对象 key。
- 组合对象至少包含 `label`、`model`、`reasoning_effort` 和 `latest`；缺失字段使用 `--`，不能阻断整个页面渲染。
- 组合列表按主组合优先、其余按接口对象顺序排列。
- 页面选择状态默认为主组合；点击任意模型卡片后，详情区域切换到该组合。

## 交互与布局

保留现有“模型 IQ” SectionCard，改造成完整的比较区：

1. 顶部显示模型组合卡片网格。每张卡片展示模型/等级标签、最新分数和状态 Tag，当前选中卡片使用明显边框或背景强调。
2. 卡片可点击并具备键盘焦点样式；点击后只更新模型 IQ 区域，不影响重置、预测、额度等其他区块。
3. 网格在桌面端使用多列，在窄屏自动降为单列或双列，长模型标签截断但保留完整 `title`。
4. 选中卡片下方显示详情指标：模型、推理等级、令牌、成本；再显示该组合的最近运行记录。
5. 当没有比较数据时回退到现有单组合展示；当请求失败时保留现有错误状态。

## 视觉与兼容性

- 沿用 Semi UI 的 `Tag`、现有 Tailwind utility class 和 `SectionCard`/`MetricCard`，不引入新依赖。
- 状态颜色继续复用 `getToneClass`，绿色、黄色、红色分别表达对应状态。
- 保留当前国际化调用方式；新增的可见文案使用现有翻译 key 或英文源 key，不在组件内新增硬编码中文。
- 只修改 `web/classic`，不改动 `web/default`。

## 验证

- 使用类型/语法检查或项目现有 lint 命令验证 JSX。
- 构建 `web/classic`，确认页面编译通过。
- 用当前 `current.json` 验证显示主组合加 8 个比较组合，共 9 张卡片；点击卡片后详情数据切换。
- 用缺失 `comparisons` 的最小数据验证回退到单组合，不抛出运行时异常。
