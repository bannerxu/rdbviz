const { createApp } = Vue;

createApp({
  data() {
    return {
      report: null,
      loading: true,
      error: "",
      charts: {},
      prefixType: "__all__",
    };
  },
  mounted() {
    this.loadDefault();
  },
  methods: {
    async loadDefault() {
      try {
        const res = await fetch("./data/report.json");
        if (!res.ok) {
          throw new Error("未找到 data/report.json，请先生成报告或使用文件选择器加载");
        }
        const data = await res.json();
        this.report = data;
        this.loading = false;
        this.$nextTick(this.renderCharts);
      } catch (e) {
        this.loading = false;
        this.error = e.message || String(e);
      }
    },
    onFile(e) {
      const file = e.target.files[0];
      if (!file) return;
      const reader = new FileReader();
      reader.onload = () => {
        try {
          this.report = JSON.parse(reader.result);
          this.error = "";
          this.loading = false;
          this.prefixType = "__all__";
          this.$nextTick(this.renderCharts);
        } catch (err) {
          this.error = "JSON 解析失败: " + err.message;
        }
      };
      reader.readAsText(file);
    },
    formatBytes(bytes) {
      if (!bytes && bytes !== 0) return "-";
      const units = ["B", "KB", "MB", "GB", "TB"];
      let v = bytes;
      let i = 0;
      while (v >= 1024 && i < units.length - 1) {
        v /= 1024;
        i++;
      }
      return v.toFixed(v < 10 && i > 0 ? 2 : 1) + " " + units[i];
    },
    formatInt(n) {
      if (n === null || n === undefined) return "-";
      return n.toLocaleString();
    },
    renderCharts() {
      if (!this.report) return;
      this.renderTypeChart();
      this.renderTTLChart();
      this.renderSizeChart();
      this.renderDBChart();
    },
    renderTypeChart() {
      const el = document.getElementById("chart-type");
      if (!el) return;
      const chart = this.getChartInstance("type", el);
      const data = this.report.types.map((t) => ({ name: t.type, value: t.size }));
      chart.setOption({
        tooltip: { trigger: "item", formatter: "{b}: {c}" },
        series: [
          {
            type: "pie",
            radius: ["35%", "70%"],
            itemStyle: { borderRadius: 6, borderColor: "#0b121d", borderWidth: 2 },
            label: { color: "#d5e3f3" },
            data,
          },
        ],
      });
    },
    renderTTLChart() {
      const el = document.getElementById("chart-ttl");
      if (!el) return;
      const chart = this.getChartInstance("ttl", el);
      const labels = this.report.ttl_buckets.map((b) => b.label);
      const values = this.report.ttl_buckets.map((b) => b.count);
      chart.setOption({
        tooltip: { trigger: "axis" },
        xAxis: { type: "category", data: labels, axisLabel: { color: "#d5e3f3" } },
        yAxis: { type: "value", axisLabel: { color: "#d5e3f3" } },
        series: [
          {
            type: "bar",
            data: values,
            itemStyle: { color: "#ff7f50", borderRadius: [6, 6, 0, 0] },
          },
        ],
        grid: { left: 40, right: 10, top: 20, bottom: 30 },
      });
    },
    renderSizeChart() {
      const el = document.getElementById("chart-size");
      if (!el) return;
      const chart = this.getChartInstance("size", el);
      const labels = this.report.size_buckets.map((b) => b.label);
      const values = this.report.size_buckets.map((b) => b.count);
      chart.setOption({
        tooltip: { trigger: "axis" },
        xAxis: { type: "category", data: labels, axisLabel: { color: "#d5e3f3" } },
        yAxis: { type: "value", axisLabel: { color: "#d5e3f3" } },
        series: [
          {
            type: "bar",
            data: values,
            itemStyle: { color: "#29d3d3", borderRadius: [6, 6, 0, 0] },
          },
        ],
        grid: { left: 40, right: 10, top: 20, bottom: 30 },
      });
    },
    renderDBChart() {
      const el = document.getElementById("chart-db");
      if (!el) return;
      const chart = this.getChartInstance("db", el);
      const dbKeys = this.report.summary.db_keys || {};
      const labels = Object.keys(dbKeys);
      const values = labels.map((k) => dbKeys[k]);
      chart.setOption({
        tooltip: { trigger: "axis" },
        xAxis: { type: "category", data: labels, axisLabel: { color: "#d5e3f3" } },
        yAxis: { type: "value", axisLabel: { color: "#d5e3f3" } },
        series: [
          {
            type: "line",
            data: values,
            smooth: true,
            lineStyle: { color: "#8a7bff", width: 3 },
            itemStyle: { color: "#8a7bff" },
            areaStyle: { color: "rgba(138,123,255,0.25)" },
          },
        ],
        grid: { left: 40, right: 10, top: 20, bottom: 30 },
      });
    },
    getChartInstance(name, el) {
      if (!this.charts[name]) {
        this.charts[name] = echarts.init(el);
        window.addEventListener("resize", () => {
          this.charts[name] && this.charts[name].resize();
        });
      }
      return this.charts[name];
    },
  },
  computed: {
    typeOptions() {
      if (!this.report) return [];
      return this.report.types.map((t) => t.type);
    },
    prefixTable() {
      if (!this.report) return [];
      if (this.prefixType === "__all__") return this.report.prefixes || [];
      const group = (this.report.prefixes_by_type || []).find((g) => g.type === this.prefixType);
      return group ? group.prefixes : [];
    },
  },
}).mount("#app");
