import { Form, Input, Modal } from "antd";
import { useEffect } from "react";

import type { SkillDetail, SkillWritePayload } from "../types";

interface SkillEditorModalProps {
  open: boolean;
  title: string;
  skill?: SkillDetail | null;
  readOnly?: boolean;
  confirmLoading?: boolean;
  onCancel: () => void;
  onSubmit: (payload: SkillWritePayload) => void;
}

interface SkillFormValues {
  slug: string;
  name: string;
  description: string;
  content: string;
}

const defaultValues: SkillFormValues = {
  slug: "",
  name: "",
  description: "",
  content: "",
};

export function SkillEditorModal({ open, title, skill, readOnly, confirmLoading, onCancel, onSubmit }: SkillEditorModalProps) {
  const [form] = Form.useForm<SkillFormValues>();

  useEffect(() => {
    if (!open) {
      return;
    }
    form.setFieldsValue(
      skill
        ? {
            slug: skill.slug,
            name: skill.name,
            description: skill.description,
            content: skill.content,
          }
        : defaultValues,
    );
  }, [form, open, skill]);

  return (
    <Modal
      title={title}
      open={open}
      onCancel={onCancel}
      onOk={() => form.submit()}
      okText={readOnly ? "关闭" : "保存"}
      cancelButtonProps={{ style: readOnly ? { display: "none" } : undefined }}
      confirmLoading={confirmLoading}
      destroyOnHidden
      width={820}
    >
      <Form<SkillFormValues>
        form={form}
        layout="vertical"
        onFinish={values => {
          if (readOnly) {
            onCancel();
            return;
          }
          onSubmit({
            slug: values.slug.trim(),
            name: values.name.trim(),
            description: values.description.trim(),
            content: values.content.trim(),
          });
        }}
      >
        <Form.Item label="Slug" name="slug" rules={[{ required: true, message: "请输入 slug" }]}>
          <Input disabled={Boolean(skill)} readOnly={readOnly} placeholder="例如：market-daily-check" />
        </Form.Item>
        <Form.Item label="名称" name="name" rules={[{ required: true, message: "请输入名称" }]}>
          <Input readOnly={readOnly} />
        </Form.Item>
        <Form.Item label="描述" name="description">
          <Input.TextArea rows={2} readOnly={readOnly} />
        </Form.Item>
        <Form.Item label="正文" name="content" rules={[{ required: true, message: "请输入正文内容" }]}>
          <Input.TextArea rows={14} readOnly={readOnly} />
        </Form.Item>
      </Form>
    </Modal>
  );
}
