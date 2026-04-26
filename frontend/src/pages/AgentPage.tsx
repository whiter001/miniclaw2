import { Alert, Button, Card, Form, Input, List, Space, Switch, Tag, Typography } from "antd";
import { startTransition, useEffect, useMemo, useState } from "react";

import { runCli, type CliRunResponse } from "../api";
import { RunResultPanel } from "../components/RunResultPanel";

interface AgentFormValues {
	prompt: string;
	workspace?: string;
	mcp?: boolean;
}

interface PromptHistoryItem {
	prompt: string;
	runAt: string;
	exitCode: number;
}

const historyStorageKey = "miniclaw-agent-history";

const promptExamples = [
	"获取当前的 skills 列表",
	"总结当前运行时状态",
	"列出与 mmx 相关的 skill 并说明用途",
];

export function AgentPage() {
	const [form] = Form.useForm<AgentFormValues>();
	const [result, setResult] = useState<CliRunResponse | null>(null);
	const [loading, setLoading] = useState(false);
	const [error, setError] = useState<string>();
	const [history, setHistory] = useState<PromptHistoryItem[]>([]);

	useEffect(() => {
		const raw = localStorage.getItem(historyStorageKey);
		if (!raw) {
			return;
		}
		try {
			setHistory(JSON.parse(raw) as PromptHistoryItem[]);
		} catch {
			localStorage.removeItem(historyStorageKey);
		}
	}, []);

	useEffect(() => {
		localStorage.setItem(historyStorageKey, JSON.stringify(history));
	}, [history]);

	const historyData = useMemo(() => history.slice(0, 6), [history]);

	async function onFinish(values: AgentFormValues) {
		setLoading(true);
		setError(undefined);
		try {
			const payload = await runCli({
				command: "agent",
				prompt: values.prompt,
				workspace: values.workspace?.trim() || undefined,
				mcp: values.mcp,
			});
			setResult(payload);
			startTransition(() => {
				setHistory(current => [{ prompt: values.prompt, runAt: payload.runAt, exitCode: payload.exitCode }, ...current].slice(0, 12));
			});
		} catch (runError) {
			setError(runError instanceof Error ? runError.message : String(runError));
		} finally {
			setLoading(false);
		}
	}

	return (
		<div className="page-stack">
			<Card className="panel-card page-intro" bordered={false}>
				<Space direction="vertical" size="small">
					<Tag color="success">Agent Console</Tag>
					<Typography.Title level={2}>执行本地 `miniclaw agent -p`</Typography.Title>
					<Typography.Paragraph type="secondary">
						这个页面会把 prompt 通过 Bun server 送到本地 CLI，适合做技能管理、状态检查和临时任务执行。
					</Typography.Paragraph>
				</Space>
			</Card>

			<div className="page-grid-two">
				<Card title="输入 Prompt" className="panel-card">
					<Form form={form} layout="vertical" initialValues={{ mcp: false }} onFinish={onFinish} className="agent-form">
						<Form.Item name="prompt" label="Prompt" rules={[{ required: true, message: "请输入要执行的 prompt。" }]}>
							<Input.TextArea rows={8} placeholder="例如：获取当前的 skills 列表" showCount maxLength={4000} />
						</Form.Item>
						<Form.Item name="workspace" label="Workspace 覆盖（可选）">
							<Input placeholder="例如：/Users/byf/.miniclaw/workspace" />
						</Form.Item>
						<Form.Item name="mcp" label="附带 --mcp" valuePropName="checked">
							<Switch />
						</Form.Item>
						<Space wrap className="command-actions">
							<Button type="primary" htmlType="submit" loading={loading}>
								执行 Agent
							</Button>
							<Button onClick={() => form.resetFields()}>清空</Button>
						</Space>
					</Form>
					<div className="example-row">
						<Typography.Text type="secondary">快速示例</Typography.Text>
						<Space wrap>
							{promptExamples.map(example => (
								<Button key={example} size="small" onClick={() => form.setFieldValue("prompt", example)}>
									{example}
								</Button>
							))}
						</Space>
					</div>
					{error ? <Alert className="agent-inline-alert" type="error" showIcon message={error} /> : null}
				</Card>

				<Card title="最近 Prompt" className="panel-card">
					<List
						dataSource={historyData}
						locale={{ emptyText: "还没有执行历史。" }}
						renderItem={item => (
							<List.Item
								actions={[
									<Button key="reuse" type="link" onClick={() => form.setFieldValue("prompt", item.prompt)}>
										复用
									</Button>,
								]}
							>
								<List.Item.Meta
									title={<Typography.Text>{item.prompt}</Typography.Text>}
									description={new Date(item.runAt).toLocaleString()}
								/>
								<Tag color={item.exitCode === 0 ? "success" : "error"}>{item.exitCode === 0 ? "成功" : "失败"}</Tag>
							</List.Item>
						)}
					/>
				</Card>
			</div>

			<RunResultPanel loading={loading} error={error} result={result} emptyText="提交 prompt 后，这里会显示 CLI 回包。" />
		</div>
	);
}