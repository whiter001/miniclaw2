import { Button, Card, Col, Row, Space, Typography } from "antd";
import { useState } from "react";

import { runCli, type CliRunRequest, type CliRunResponse } from "../api";
import { RunResultPanel } from "../components/RunResultPanel";
import { emitHistoryUpdated, historyStorageKey, readPromptHistory, type PromptHistoryItem } from "../workbenchEvents";

interface QuickCommand {
	key: string;
	title: string;
	description: string;
	reusePrompt: string;
	request: CliRunRequest;
}

const quickCommands: QuickCommand[] = [
	{
		key: "status",
		title: "查看运行状态",
		description: "执行 miniclaw status，检查当前配置、workspace 和渠道状态。",
		reusePrompt: "总结当前运行时状态，并指出异常项和下一步建议",
		request: { command: "status" },
	},
	{
		key: "skills",
		title: "列出当前 skills",
		description: "通过 agent prompt 获取当前工作区中的 skill 清单。",
		reusePrompt: "获取当前的 skills 列表",
		request: { command: "agent", prompt: "获取当前的 skills 列表" },
	},
	{
		key: "mmx",
		title: "查看 mmx 相关 skill",
		description: "检查运行时中与 mmx 相关的 skill 和用途。",
		reusePrompt: "列出与 mmx 相关的 skill 并说明用途",
		request: { command: "agent", prompt: "列出与 mmx 相关的 skill 并说明用途" },
	},
];

function persistPromptHistory(item: PromptHistoryItem, limit = 12) {
	const nextHistory = [item, ...readPromptHistory()].slice(0, limit);
	if (typeof window !== "undefined") {
		window.localStorage.setItem(historyStorageKey, JSON.stringify(nextHistory));
		emitHistoryUpdated(nextHistory);
	}
	return nextHistory;
}

export function CommandsPage() {
	const [runningKey, setRunningKey] = useState<string>();
	const [lastCommandTitle, setLastCommandTitle] = useState<string>();
	const [error, setError] = useState<string>();
	const [result, setResult] = useState<CliRunResponse | null>(null);

	async function execute(item: QuickCommand) {
		setRunningKey(item.key);
		setLastCommandTitle(item.title);
		setError(undefined);
		try {
			const payload = await runCli(item.request);
			setResult(payload);
			persistPromptHistory({
				title: item.title,
				prompt: item.reusePrompt,
				source: "command",
				runAt: payload.runAt,
				exitCode: payload.exitCode,
				durationMs: payload.durationMs,
				commandLine: payload.commandLine,
			});
		} catch (runError) {
			setError(runError instanceof Error ? runError.message : String(runError));
		} finally {
			setRunningKey(undefined);
		}
	}

	return (
		<div className="page-stack">
			<Card className="panel-card page-intro" bordered={false}>
				<Space direction="vertical" size="small">
					<Typography.Title level={2}>快捷命令面板</Typography.Title>
					<Typography.Paragraph type="secondary">
						这里放的是前端直接要用到的几类常见命令，执行后会同步写入最近任务，避免每次都回到 Agent 页手输 prompt。
					</Typography.Paragraph>
				</Space>
			</Card>

			<Row gutter={[16, 16]}>
				{quickCommands.map(item => (
					<Col xs={24} md={12} xl={8} key={item.key}>
						<Card className="panel-card preset-card" title={item.title}>
							<Space direction="vertical" size="middle" className="preset-card-body">
								<Typography.Paragraph type="secondary">{item.description}</Typography.Paragraph>
								<Button
									type="primary"
									onClick={() => void execute(item)}
									loading={runningKey === item.key}
									disabled={Boolean(runningKey && runningKey !== item.key)}
								>
									立即执行
								</Button>
							</Space>
						</Card>
					</Col>
				))}
			</Row>

			<RunResultPanel
				title={lastCommandTitle ? `执行结果 · ${lastCommandTitle}` : "执行结果"}
				loading={Boolean(runningKey)}
				error={error}
				result={result}
				emptyText="点击上面的快捷命令后，这里会显示结果。"
			/>
		</div>
	);
}