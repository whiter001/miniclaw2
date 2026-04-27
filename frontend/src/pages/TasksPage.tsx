import { Alert, Button, Card, Popconfirm, Space, Table, Typography, message } from "antd";
import { useEffect, useState } from "react";

import { createTask, deleteTask as removeTask, listTasks, runTask, updateTask } from "../api";
import { ellipsis, formatDateTime } from "../format";
import { StatusTag } from "../components/StatusTag";
import { TaskEditorModal } from "../components/TaskEditorModal";
import type { TaskRecord, TaskWritePayload } from "../types";

export function TasksPage() {
  const [tasks, setTasks] = useState<TaskRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [runningId, setRunningId] = useState<string>();
  const [open, setOpen] = useState(false);
  const [editingTask, setEditingTask] = useState<TaskRecord | null>(null);

  async function load() {
    setLoading(true);
    try {
      setTasks(await listTasks());
    } catch (error) {
      message.error(error instanceof Error ? error.message : String(error));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function handleSubmit(payload: TaskWritePayload) {
    setSaving(true);
    try {
      if (editingTask) {
        await updateTask(editingTask.id, payload);
        message.success("任务已更新");
      } else {
        await createTask(payload);
        message.success("任务已创建");
      }
      setOpen(false);
      setEditingTask(null);
      await load();
    } catch (error) {
      message.error(error instanceof Error ? error.message : String(error));
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete(taskId: string) {
    try {
      await removeTask(taskId);
      message.success("任务已删除");
      await load();
    } catch (error) {
      message.error(error instanceof Error ? error.message : String(error));
    }
  }

  async function handleRun(taskId: string) {
    setRunningId(taskId);
    try {
      const result = await runTask(taskId);
      message[result.exitCode === 0 ? "success" : "error"](result.exitCode === 0 ? "任务已触发执行" : "任务执行返回错误");
      await load();
    } catch (error) {
      message.error(error instanceof Error ? error.message : String(error));
    } finally {
      setRunningId(undefined);
    }
  }

  return (
    <div className="page-section">
      <div className="page-actions">
        <div>
          <Typography.Title level={4} className="page-title">
            任务管理
          </Typography.Title>
          <Typography.Text type="secondary" className="page-subtitle">
            这里管理的是工作区中的 cron 任务定义，包含任务 Prompt、本地运行策略和启停状态。
          </Typography.Text>
        </div>
        <Button
          type="primary"
          onClick={() => {
            setEditingTask(null);
            setOpen(true);
          }}
        >
          新建任务
        </Button>
      </div>

      <Alert type="info" showIcon message="当前页和“定时任务”页共用同一份 workspace cron 数据：这里偏重任务定义和执行入口。" />

      <Card size="small" variant="outlined">
        <Table<TaskRecord>
          rowKey="id"
          size="small"
          loading={loading}
          dataSource={tasks}
          columns={[
            { title: "任务 ID", dataIndex: "id" },
            {
              title: "状态",
              render: (_value, record) => <StatusTag value={record.enabled ? "enabled" : "disabled"} />,
            },
            {
              title: "最近结果",
              render: (_value, record) => (record.lastStatus ? <StatusTag value={record.lastStatus} /> : <span className="muted">-</span>),
            },
            { title: "Cron", dataIndex: "schedule" },
            {
              title: "Prompt",
              dataIndex: "prompt",
              render: (value: string) => ellipsis(value, 68),
            },
            {
              title: "最近执行",
              dataIndex: "lastFinishedAt",
              render: (value: string | null) => formatDateTime(value),
            },
            {
              title: "操作",
              render: (_value, record) => (
                <Space size={4}>
                  <Button size="small" onClick={() => {
                    setEditingTask(record);
                    setOpen(true);
                  }}>
                    编辑
                  </Button>
                  <Button size="small" type="primary" loading={runningId === record.id} onClick={() => void handleRun(record.id)}>
                    立即执行
                  </Button>
                  <Popconfirm title="删除任务" description="删除后会同时移除对应的状态文件。" onConfirm={() => void handleDelete(record.id)}>
                    <Button size="small" danger>
                      删除
                    </Button>
                  </Popconfirm>
                </Space>
              ),
            },
          ]}
        />
      </Card>

      <TaskEditorModal
        open={open}
        title={editingTask ? "编辑任务" : "新建任务"}
        task={editingTask}
        confirmLoading={saving}
        onCancel={() => {
          setOpen(false);
          setEditingTask(null);
        }}
        onSubmit={payload => void handleSubmit(payload)}
      />
    </div>
  );
}
