# shadcn/ui 全量迁移计划（已完成）

- 分支：`feat/shadcn-ui-full-migration-20260219`
- 日期：`2026-02-19`
- 目标：将 `vite-frontend` 从 HeroUI 彻底迁移到 shadcn/ui（含依赖、Provider、组件实现与构建验证）
- 说明：用户提到的 `shadcu-ui` 按 `shadcn/ui` 执行

## 0. 现状基线（已完成）

- HeroUI 直接使用文件：`22` 个（`src/` 下）
- HeroUI 组件族：`button/card/input/select/modal/table/chip/spinner/switch/alert/accordion/checkbox/dropdown/tabs/radio/date-picker/progress/navbar/link/system/use-theme` 等
- 关键复杂页：`forward.tsx`、`tunnel.tsx`、`node.tsx`、`user.tsx`
- 语义色类大量依赖：`text-default-*`、`bg-primary-*`、`border-divider`、`text-foreground` 等

## 1. 执行步骤

状态标记：`[ ] 未开始` / `[-] 进行中` / `[x] 已完成`

- [x] S1. 创建迁移分支并冻结迁移范围（仅 `vite-frontend`）
- [x] S2. 完成全量使用点扫描（Grep/rg/AST + 官方文档检索）
- [x] S3. 建立 shadcn/ui 基础设施（`components.json`、`src/lib/utils.ts`、`src/components/ui/*` 基础原子组件）
- [x] S4. 建立 HeroUI -> shadcn 兼容桥接层（`src/shadcn-bridge/heroui/*`）并替换全部页面导入
- [x] S5. 迁移全局 Provider/主题能力（替换 `HeroUIProvider`、`useTheme`、`useDisclosure`）
- [x] S6. 替换 Tailwind 主题来源（移除 HeroUI 主题插件，补齐语义色 token 与兼容工具类）
- [x] S7. 移除 HeroUI 依赖并修复构建（`npm install` + `npm run build`）
- [x] S8. 回写完成记录与验收（确认无 `@heroui/*` 运行时依赖）

## 2. 组件映射策略（本次执行）

- Button -> shadcn `button` + 兼容 `isLoading/isIconOnly/startContent/endContent/onPress`
- Input/Textarea -> shadcn `input/textarea` + label/description/error 容器
- Modal -> shadcn `dialog`（兼容 `isOpen/onOpenChange` 与 Header/Body/Footer 插槽）
- Select -> shadcn `select`（单选）+ 命令式多选兼容实现（多选场景）
- Table -> shadcn `table`（兼容 `items + render function + empty/loading`）
- Dropdown/Tabs/Radio/Switch/Checkbox/Accordion/Alert/Progress/Card/Separator -> 对应 shadcn 组件封装
- DatePicker -> 基于原生日期输入 + 兼容 value/onChange 的桥接实现（保留现有业务数据结构）

## 3. 执行记录（每步完成即更新）

- [2026-02-19] 完成 S1：创建分支 `feat/shadcn-ui-full-migration-20260219`
- [2026-02-19] 完成 S2：完成 HeroUI 使用点与迁移风险扫描；确认迁移顺序
- [2026-02-19] 完成 S3：新增 `components.json`、`src/lib/utils.ts` 与 `src/components/ui/*`（button/dialog/dropdown/select/table/checkbox/switch/tabs/accordion/progress 等）
- [2026-02-19] 完成 S4：新增 `src/shadcn-bridge/heroui/*` 兼容桥接层，并将现网全部导入替换为 `@/shadcn-bridge/heroui/*`
- [2026-02-19] 完成 S5：通过桥接层接管 `HeroUIProvider`、`useTheme`、`useDisclosure`，保持页面业务逻辑不改动
- [2026-02-19] 完成 S6：移除 `@heroui/theme` Tailwind 插件，改为本地 token 体系（`tailwind.config.js` + `src/styles/globals.css`）
- [2026-02-19] 完成 S7：删除全部 HeroUI/NextUI 依赖，补齐 `@internationalized/date` 与 `@react-aria/i18n` 显式依赖
- [2026-02-19] 完成 S8：构建验收通过（`npm run build`），`package.json` 已无 `heroui/nextui` 依赖
- [2026-02-19] 验证结果：业务代码中 `@heroui/*` 导入为 `0`，已统一替换为 `@/shadcn-bridge/heroui/*`（22 文件，106 处）
- [2026-02-19] 后续修复：`src/components/ui/button.tsx` 与 `src/shadcn-bridge/heroui/button.tsx` 改为 `forwardRef`，消除 `DropdownMenuTrigger asChild` 场景 ref 警告；复构建通过
