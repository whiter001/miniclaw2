import { Alert, Card, Descriptions, Empty, Spin, Tabs, Tag, Typography } from "antd";

import type { CliRunResponse } from "../api";

interface RunResultPanelProps {
	loading: boolean;
	error?: string;
	result?: CliRunResponse | null;
	title?: string;
	emptyText?: string;
}

export function RunResultPanel({ loading, error, result, title = "执行结果", emptyText = "尚未执行命令。" }: RunResultPanelProps) {
	return (
		<Card title={title} className="panel-card result-panel">
			{loading ? (
				<div className="panel-loading">
					<Spin size="large" />
					<Typography.Text type="secondary">命令执行中，稍候返回结果。</Typography.Text>
				</div>
			) : error ? (
				<Alert type="error" showIcon message="执行失败" description={error} />
			) : !result ? (
				<Empty description={emptyText} />
			) : (
				<div className="result-stack">
					<Descriptions column={1} size="small" className="result-meta">
						<Descriptions.Item label="命令">
							<Typography.Text code>{result.commandLine}</Typography.Text>
						</Descriptions.Item>
						<Descriptions.Item label="退出码">
							<Tag color={result.exitCode === 0 ? "success" : "error"}>{result.exitCode}</Tag>
						</Descriptions.Item>
						<Descriptions.Item label="耗时">{result.durationMs} ms</Descriptions.Item>
						<Descriptions.Item label="CLI 模式">
							<Tag color={result.binaryMode === "binary" ? "processing" : "warning"}>{result.binaryMode}</Tag>
						</Descriptions.Item>
					</Descriptions>
					<Tabs
						className="result-tabs"
						items={[
							{
								key: "stdout",
								label: "stdout",
								children: <pre className="terminal-output">{result.stdout || "(empty)"}</pre>,
							},
							{
								key: "stderr",
								label: "stderr",
								children: <pre className="terminal-output terminal-output-muted">{result.stderr || "(empty)"}</pre>,
							},
						]}
					/>
				</div>
			)}
		</Card>
	);
}