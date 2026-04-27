import { Alert, Button, Card, Popconfirm, Space, Switch, Table, Typography, message } from "antd";
import { useEffect, useState } from "react";

import { createTask, deleteTask as removeTask, listTasks, runTask, updateTask } from "../api";
import { formatDateTime, taskToPayload } from "../format";
import { StatusTag } from "../components/StatusTag";
import { TaskEditorModal } from "../components/TaskEditorModal";
import type { TaskRecord, TaskWritePayload } from "../types";

export function SchedulesPage() {
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
        message.success("定时任务已更新");
      } else {
        await createTask(payload);
        message.success("定时任务已创建");
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

  async function handleToggle(task: TaskRecord) {
    try {
      await updateTask(task.id, { ...taskToPayload(task), enabled: !task.enabled });
      await load();
    } catch (error) {
      message.error(error instanceof Error ? error.message : String(error));
    }
  }

  async function handleRun(taskId: string) {
    setRunningId(taskId);
    try {
      const result = await runTask(taskId);
      message[result.exitCode === 0 ? "success" : "error"](result.exitCode === 0 ? "已手动触发" : "触发失败");
      await load();
    } catch (error) {
      message.error(error instanceof Error ? error.message : String(error));
    } finally {
      setRunningId(undefined);
    }
  }

  async function handleDelete(taskId: string) {
    try {
      await removeTask(taskId);
      message.success("定时任务已删除");
      await load();
    } catch (error) {
      message.error(error instanceof Error ? error.message : String(error));
    }
  }

  return (
    <div className="page-section">
      <div className="page-actions">
        <div>
          <Typography.Title level={4} className="page-title">
            定时任务
          </Typography.Title>
          <Typography.Text type="secondary" className="page-subtitle">
            关注 cron 调度状态、是否到期、最近一次运行结果，以及下次执行时间。
          </Typography.Text>
        </div>
        <Button type="primary" onClick={() => {
          setEditingTask(null);
          setOpen(true);
        }}>
          新建定时任务
        </Button>
      </div>

      <Alert type="info" showIcon message="如果工作区中已有 state/cron/tasks 文件，这里会直接反映最近执行结果和 session 文件。" />

      <Card size="small" variant="outlined">
        <Table<TaskRecord>
          rowKey="id"
          size="small"
          loading={loading}
          dataSource={tasks}
          columns={[
            { title: "任务", dataIndex: "id" },
            { title: "Cron", dataIndex: "schedule" },
            {
              title: "状态",
              render: (_value, record) => <StatusTag value={record.enabled ? "enabled" : "disabled"} />,
            },
            {
              title: "运行中",
              render: (_value, record) => <StatusTag value={record.running ? "running" : "idle"} />,
            },
            {
              title: "待执行",
              render: (_value, record) => (record.due ? <StatusTag value="due" /> : <span className="muted">否</span>),
            },
            {
              title: "下次执行",
              dataIndex: "nextRunAt",
              render: (value: string | null) => formatDateTime(value),
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
                  <Switch checked={record.enabled} onChange={() => void handleToggle(record)} />
                  <Button size="small" onClick={() => {
                    setEditingTask(record);
                    setOpen(true);
                  }}>
                    编辑
                  </Button>
                  <Button size="small" type="primary" loading={runningId === record.id} onClick={() => void handleRun(record.id)}>
                    立即触发
                  </Button>
                  <Popconfirm title="删除定时任务" description="删除后不会保留当前任务文件。" onConfirm={() => void handleDelete(record.id)}>
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
        title={editingTask ? "编辑定时任务" : "新建定时任务"}
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
