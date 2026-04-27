import { Button, Card, Popconfirm, Space, Table, Typography, message } from "antd";
import { useEffect, useState } from "react";

import { createSkill, deleteSkill as removeSkill, getSkillDetail, listSkills, updateSkill } from "../api";
import { ellipsis, formatDateTime } from "../format";
import { SkillEditorModal } from "../components/SkillEditorModal";
import type { SkillDetail, SkillRecord, SkillWritePayload } from "../types";

export function SkillsPage() {
  const [skills, setSkills] = useState<SkillRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [open, setOpen] = useState(false);
  const [currentSkill, setCurrentSkill] = useState<SkillDetail | null>(null);
  const [readOnly, setReadOnly] = useState(false);

  async function load() {
    setLoading(true);
    try {
      setSkills(await listSkills());
    } catch (error) {
      message.error(error instanceof Error ? error.message : String(error));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function openSkill(slug: string, viewOnly: boolean) {
    try {
      setCurrentSkill(await getSkillDetail(slug));
      setReadOnly(viewOnly);
      setOpen(true);
    } catch (error) {
      message.error(error instanceof Error ? error.message : String(error));
    }
  }

  async function handleSubmit(payload: SkillWritePayload) {
    setSaving(true);
    try {
      if (currentSkill) {
        await updateSkill(currentSkill.slug, payload);
        message.success("Skill 已更新");
      } else {
        await createSkill(payload);
        message.success("Skill 已创建");
      }
      setOpen(false);
      setCurrentSkill(null);
      setReadOnly(false);
      await load();
    } catch (error) {
      message.error(error instanceof Error ? error.message : String(error));
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete(slug: string) {
    try {
      await removeSkill(slug);
      message.success("Skill 已删除");
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
            Skills
          </Typography.Title>
          <Typography.Text type="secondary" className="page-subtitle">
            直接扫描 workspace skills 目录，支持查看、新建、编辑和删除自定义 Skill。
          </Typography.Text>
        </div>
        <Space size={8}>
          <Button onClick={() => void load()}>刷新</Button>
          <Button
            type="primary"
            onClick={() => {
              setCurrentSkill(null);
              setReadOnly(false);
              setOpen(true);
            }}
          >
            新建 Skill
          </Button>
        </Space>
      </div>

      <Card size="small" variant="outlined">
        <Table<SkillRecord>
          rowKey="slug"
          size="small"
          loading={loading}
          dataSource={skills}
          columns={[
            { title: "名称", dataIndex: "name" },
            { title: "Slug", dataIndex: "slug" },
            {
              title: "描述",
              dataIndex: "description",
              render: (value: string) => ellipsis(value, 68),
            },
            {
              title: "更新时间",
              dataIndex: "updatedAt",
              render: (value: string) => formatDateTime(value),
            },
            {
              title: "路径",
              dataIndex: "skillFilePath",
              render: (value: string) => ellipsis(value, 48),
            },
            {
              title: "操作",
              render: (_value, record) => (
                <Space size={4}>
                  <Button size="small" onClick={() => void openSkill(record.slug, true)}>
                    查看
                  </Button>
                  <Button size="small" onClick={() => void openSkill(record.slug, false)}>
                    编辑
                  </Button>
                  <Popconfirm title="删除 Skill" description="会直接删除该 Skill 目录。" onConfirm={() => void handleDelete(record.slug)}>
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

      <SkillEditorModal
        open={open}
        title={currentSkill ? (readOnly ? "查看 Skill" : "编辑 Skill") : "新建 Skill"}
        skill={currentSkill}
        readOnly={readOnly}
        confirmLoading={saving}
        onCancel={() => {
          setOpen(false);
          setCurrentSkill(null);
          setReadOnly(false);
        }}
        onSubmit={payload => void handleSubmit(payload)}
      />
    </div>
  );
}
