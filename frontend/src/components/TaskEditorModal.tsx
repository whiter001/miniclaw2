import { Form, Input, InputNumber, Modal, Switch } from "antd";
import { useEffect } from "react";

import type { TaskRecord, TaskWritePayload } from "../types";

interface TaskEditorModalProps {
  open: boolean;
  title: string;
  task?: TaskRecord | null;
  confirmLoading?: boolean;
  onCancel: () => void;
  onSubmit: (payload: TaskWritePayload) => void;
}

interface TaskFormValues {
  id: string;
  description: string;
  schedule: string;
  prompt: string;
  enabled: boolean;
  skipIfRunning: boolean;
  timeoutSeconds: number;
  maxToolIterations: number;
  enableMcp: boolean;
}

const defaultValues: TaskFormValues = {
  id: "",
  description: "",
  schedule: "*/10 * * * *",
  prompt: "",
  enabled: true,
  skipIfRunning: true,
  timeoutSeconds: 600,
  maxToolIterations: 0,
  enableMcp: false,
};

export function TaskEditorModal({ open, title, task, confirmLoading, onCancel, onSubmit }: TaskEditorModalProps) {
  const [form] = Form.useForm<TaskFormValues>();

  useEffect(() => {
    if (!open) {
      return;
    }
    form.setFieldsValue(
      task
        ? {
            id: task.id,
            description: task.description,
            schedule: task.schedule,
            prompt: task.prompt,
            enabled: task.enabled,
            skipIfRunning: task.skipIfRunning,
            timeoutSeconds: task.timeoutSeconds,
            maxToolIterations: task.maxToolIterations,
            enableMcp: task.enableMcp ?? false,
          }
        : defaultValues,
    );
  }, [form, open, task]);

  return (
    <Modal
      title={title}
      open={open}
      onCancel={onCancel}
      onOk={() => form.submit()}
      confirmLoading={confirmLoading}
      destroyOnHidden
      width={760}
    >
      <Form<TaskFormValues>
        form={form}
        layout="vertical"
        onFinish={values =>
          onSubmit({
            id: values.id.trim(),
            description: values.description.trim(),
            schedule: values.schedule.trim(),
            prompt: values.prompt.trim(),
            enabled: values.enabled,
            skipIfRunning: values.skipIfRunning,
            timeoutSeconds: values.timeoutSeconds,
            maxToolIterations: values.maxToolIterations,
            enableMcp: values.enableMcp,
          })
        }
      >
        <Form.Item label="任务 ID" name="id" rules={[{ required: true, message: "请输入任务 ID" }]}>
          <Input disabled={Boolean(task)} placeholder="例如：daily-report" />
        </Form.Item>
        <Form.Item label="描述" name="description">
          <Input.TextArea rows={2} placeholder="简要描述任务目标" />
        </Form.Item>
        <Form.Item label="Cron 表达式" name="schedule" rules={[{ required: true, message: "请输入 Cron 表达式" }]}>
          <Input placeholder="例如：*/10 * * * * 或 @daily" />
        </Form.Item>
        <Form.Item label="任务 Prompt" name="prompt" rules={[{ required: true, message: "请输入任务 Prompt" }]}>
          <Input.TextArea rows={5} placeholder="输入需要 agent 执行的任务内容" />
        </Form.Item>
        <div className="form-grid-two">
          <Form.Item label="超时秒数" name="timeoutSeconds">
            <InputNumber min={1} style={{ width: "100%" }} />
          </Form.Item>
          <Form.Item label="最大工具迭代" name="maxToolIterations">
            <InputNumber min={0} style={{ width: "100%" }} />
          </Form.Item>
        </div>
        <div className="form-grid-three">
          <Form.Item label="启用任务" name="enabled" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item label="运行中跳过" name="skipIfRunning" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item label="启用 MCP" name="enableMcp" valuePropName="checked">
            <Switch />
          </Form.Item>
        </div>
      </Form>
    </Modal>
  );
}
