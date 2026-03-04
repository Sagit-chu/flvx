# Session Context

Session ID: ses_34d7fcdb8ffekb7lvyJwVvoS8T
Commit Message: 增加功能：转发规则批量导入格式，兼容ny的导入格式
ny格式如下：
{"dest":["151.241.129.52:23609"],"list

## Prompts

### Prompt 1

增加功能：转发规则批量导入格式，兼容ny的导入格式
ny格式如下：
{"dest":["151.241.129.52:23609"],"listen_port":20224,"name":"灵玥-JP-Lpt【三网通用】"}{"dest":["64.81.33.2:24577"],"listen_port":41034,"name":"Yolo-US-Lpt【三网通用】"}

ny导入的时候是选择入口的，flvx转发导入是选择隧道的，需要兼容一下，请设计计划

## Summary

计划文档已设计完成。以下是计划摘要：

## 计划摘要

### 格式对比
| 特性 | flvx格式 | ny格式 |
|------|----------|--------|
| 格式 | 管道分隔文本 | 紧凑JSON对象 |
| 示例 | `ip:port\|name\|port` | `{"dest":["ip:port"],"listen_port":port,"name":"name"}` |
| 多目标 | 逗号分隔 | dest数组 |
| 选择器 | 选择隧道 | 选择入口节点→匹配隧道 |

### 核心设计
1. **格式选择器**：手动选择"flvx格式"或"ny格式"
2. **ny格式流程**：选择入口节点 → 匹配端口转发隧道 → 匹配0个报错、1个直接用、多个弹窗选
3. **不自动创建隧道**：无匹配时提示用户先创建

### 任务清单
1. 新增`importFormat`、`selectedEntryNode`、`matchedTunnels`等状态
2. 重构导入模态框UI，添加格式选择器
3. 实现ny格式紧凑JSON解析函数
4. 实现入口节点选择与隧道匹配逻辑
5. 多隧道匹配时的选择器弹窗
6. 整合ny格式到`executeImport`函数
7. 更新提示文案

### 改动范围
- `vite-frontend/src/pages/forward.tsx` - 主要改动文件

是否需要我调整计划内容？
