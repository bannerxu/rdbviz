# RDB 可视化分析使用方案

本文档描述如何在不加载 Redis 的情况下，以低内存方式解析 RDB，并通过 Vue 页面可视化展示统计结果。

## 方案概述

- 解析：Go + `github.com/hdt3213/rdb`（流式解析，低内存）
- 输出：`report.json`
- 展示：Vue + ECharts（静态页面）

目录结构：

```
.
├── dump.rdb
├── rdbviz-tool
│   ├── go.mod
│   └── main.go
└── rdbviz
    ├── index.html
    ├── app.js
    ├── style.css
    └── data
        └── report.json
```

## 生成报告

进入解析器目录并执行：

```bash
cd rdbviz-tool

go mod tidy

go run . \
  -rdb ../dump.rdb \
  -out ../rdbviz/data/report.json \
  -prefix-sep : \
  -prefix-depth 2 \
  -topn 50 \
  -progress 5s
```

参数说明：

- `-rdb`：RDB 文件路径
- `-out`：输出报告路径（JSON）
- `-prefix-sep`：前缀分隔符，默认 `:`
- `-prefix-depth`：前缀统计最大深度，默认 `2`
- `-topn`：TopN 数量，默认 `50`
- `-progress`：进度输出间隔，默认 `5s`，设置为 `0` 关闭

## 启动可视化页面

```bash
cd rdbviz
python3 -m http.server 8080
```

浏览器访问 `http://localhost:8080`。

也可以不启动服务，直接在页面上选择 `report.json` 文件加载。

## 输出内容

- 总 key 数、总大小、DB 分布
- 类型占比（按大小）
- TTL 分布
- Key 大小分布
- 前缀 TopN（按大小，可按类型筛选）
- BigKey TopN（按大小）

## 常见问题

- 如果解析失败：确认 `dump.rdb` 是否完整、是否为 Redis 7.x 版本导出。
- 如果页面加载失败：确认 `rdbviz/data/report.json` 是否存在。

