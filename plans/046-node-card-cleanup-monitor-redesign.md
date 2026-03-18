# 046 - Node Card Cleanup & Monitor Redesign

## Tasks

- [x] 1. Remove duplicate system metrics from node cards in `node.tsx` (CPU, memory, upload/download speed, upload/download traffic, disk, load)
- [x] 2. Redesign monitor `ServerCard` in `monitor-view.tsx` to match the node card style from `node.tsx`
- [x] 3. Make all charts in monitor view update incrementally (streaming) instead of full page reload
