import { Tag } from "antd";

const statusMap: Record<string, { color: string; label: string }> = {
  success: { color: "success", label: "成功" },
  failed: { color: "error", label: "失败" },
  running: { color: "processing", label: "运行中" },
  unknown: { color: "default", label: "未知" },
  enabled: { color: "success", label: "启用" },
  disabled: { color: "default", label: "停用" },
  cron: { color: "processing", label: "定时" },
  agent: { color: "geekblue", label: "手动" },
  due: { color: "warning", label: "待执行" },
  idle: { color: "default", label: "空闲" },
};

export function StatusTag({ value }: { value: string }) {
  const resolved = statusMap[value] ?? { color: "default", label: value || "-" };
  return <Tag color={resolved.color}>{resolved.label}</Tag>;
}
