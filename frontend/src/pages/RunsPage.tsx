import { Button, Card, Descriptions, Drawer, Space, Table, Typography, message } from "antd";
import { useEffect, useMemo, useState } from "react";

import { getRunDetail, listRuns } from "../api";
import { ellipsis, formatDateTime } from "../format";
import { StatusTag } from "../components/StatusTag";
import type { RunDetail, RunMessage, RunRecord } from "../types";

function renderMessages(messages: RunMessage[]) {
  return messages
    .map(message => {
      const header = [formatDateTime(message.ts), message.kind, message.role, message.toolName].filter(Boolean).join(" · ");
      return `${header}\n${message.content || "(empty)"}`;
    })
    .join("\n\n");
}

export function RunsPage() {
  const [runs, setRuns] = useState<RunRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [detailLoading, setDetailLoading] = useState(false);
  const [selectedRun, setSelectedRun] = useState<RunDetail | null>(null);

  async function load() {
    setLoading(true);
    try {
      setRuns(await listRuns());
    } catch (error) {
      message.error(error instanceof Error ? error.message : String(error));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function openDetail(runId: string) {
    setDetailLoading(true);
    setDrawerOpen(true);
    try {
      setSelectedRun(await getRunDetail(runId));
    } catch (error) {
      message.error(error instanceof Error ? error.message : String(error));
      setDrawerOpen(false);
    } finally {
      setDetailLoading(false);
    }
  }

  const messageDump = useMemo(() => renderMessages(selectedRun?.messages ?? []), [selectedRun]);

  return (
    <div className="page-section">
      <div className="page-actions">
        <div>
          <Typography.Title level={4} className="page-title">
            执行记录
          </Typography.Title>
          <Typography.Text type="secondary" className="page-subtitle">
            基于工作区 sessions 文件汇总最近运行，支持查看逐条消息和工具调用痕迹。
          </Typography.Text>
        </div>
        <Button onClick={() => void load()}>刷新</Button>
      </div>

      <Card size="small" variant="outlined">
        <Table<RunRecord>
          rowKey="id"
          size="small"
          loading={loading}
          dataSource={runs}
          columns={[
            {
              title: "执行项",
              render: (_value, record) => (
                <div>
                  <Typography.Text strong>{record.title}</Typography.Text>
                  <div className="muted">{ellipsis(record.prompt, 42)}</div>
                </div>
              ),
            },
            {
              title: "来源",
              dataIndex: "source",
              render: (value: string) => <StatusTag value={value} />,
            },
            {
              title: "状态",
              dataIndex: "status",
              render: (value: string) => <StatusTag value={value} />,
            },
            { title: "开始时间", dataIndex: "createdAt", render: (value: string) => formatDateTime(value) },
            { title: "结束时间", dataIndex: "finishedAt", render: (value: string | null) => formatDateTime(value) },
            {
              title: "摘要",
              dataIndex: "summary",
              render: (value: string) => ellipsis(value, 64),
            },
            {
              title: "操作",
              render: (_value, record) => (
                <Space size={4}>
                  <Button size="small" onClick={() => void openDetail(record.id)}>
                    查看详情
                  </Button>
                </Space>
              ),
            },
          ]}
        />
      </Card>

      <Drawer open={drawerOpen} title={selectedRun?.title || "执行详情"} width={860} onClose={() => setDrawerOpen(false)} loading={detailLoading}>
        {selectedRun ? (
          <div className="page-section">
            <Descriptions column={1} bordered size="small">
              <Descriptions.Item label="Run ID">{selectedRun.id}</Descriptions.Item>
              <Descriptions.Item label="来源">{selectedRun.source}</Descriptions.Item>
              <Descriptions.Item label="状态">{selectedRun.status}</Descriptions.Item>
              <Descriptions.Item label="任务 ID">{selectedRun.taskId || "-"}</Descriptions.Item>
              <Descriptions.Item label="开始时间">{formatDateTime(selectedRun.createdAt)}</Descriptions.Item>
              <Descriptions.Item label="结束时间">{formatDateTime(selectedRun.finishedAt)}</Descriptions.Item>
              <Descriptions.Item label="Session 文件">{selectedRun.sessionFile}</Descriptions.Item>
            </Descriptions>
            <Card size="small" title="摘要" variant="outlined">
              <Typography.Paragraph style={{ marginBottom: 0 }}>{selectedRun.summary || "-"}</Typography.Paragraph>
            </Card>
            <Card size="small" title="消息流" variant="outlined">
              <pre className="code-block">{messageDump || "暂无消息"}</pre>
            </Card>
          </div>
        ) : null}
      </Drawer>
    </div>
  );
}
