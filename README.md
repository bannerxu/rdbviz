# RDB 可视化分析

低内存、流式解析 Redis RDB 文件，并通过 Vue 页面可视化展示 key 分布、TTL 分布、BigKey、前缀统计等。

## 功能

- 总 key 数 / 总大小 / DB 分布
- 类型占比（按大小）
- TTL 分布
- Key 大小分布
- 前缀 TopN（按大小，可按类型筛选）
- BigKey TopN（按大小）

## 使用方式

### 1. 生成报告（JSON）

```bash
cd /Users/xgl/GolandProjects/rdr/rdbviz-tool

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

### 2. 启动可视化页面

```bash
cd /Users/xgl/GolandProjects/rdr/rdbviz
python3 -m http.server 8080
```

浏览器访问 `http://localhost:8080`。

也可以不启动服务，直接在页面上选择 `report.json` 文件加载。

## 注意事项

- 本项目默认忽略 `dump.rdb` 与 `rdbviz/data/report.json`（见 `.gitignore`）。
- 解析采用流式方式，不需要把 RDB 加载到 Redis，内存占用较低。

