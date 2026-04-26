import { Alert, Button, Card, Collapse, Descriptions, Empty, Input, Row, Col, Statistic, Switch, Tag, Typography } from "antd";
import { startTransition, useEffect, useMemo, useState, type KeyboardEvent } from "react";

import { runCli, type CliRunResponse } from "../api";
import { RunResultPanel } from "../components/RunResultPanel";
import { useWorkspaceStatus } from "../workspaceStatus";
import {
	clearRequestedPrompt,
	consumeRequestedPrompt,
	emitHistoryUpdated,
	historyStorageKey,
	historyUpdatedEvent,
	promptRequestedEvent,
	readPromptHistory,
	type HistoryUpdatedDetail,
	type PromptHistoryItem,
	type PromptRequestedDetail,
} from "../workbenchEvents";

const observedSections = ["composer", "scenes", "history", "workspace"] as const;

export type AgentSectionKey = (typeof observedSections)[number];

interface AgentPageProps {
	requestedSection?: AgentSectionKey;
	onSectionActive?: (sectionId: AgentSectionKey) => void;
}

function isAgentSectionKey(value: string): value is AgentSectionKey {
	return observedSections.some(sectionId => sectionId === value);
}

const sceneTemplates = [
	{
		key: "skills",
		title: "技能盘点",
		eyebrow: "快速查看",
		description: "快速查看当前 workspace 中有哪些 skills。",
		prompt: "获取当前的 skills 列表",
	},
	{
		key: "status",
		title: "运行时巡检",
		eyebrow: "状态总结",
		description: "总结当前运行状态、工作区和渠道配置。",
		prompt: "总结当前运行时状态",
	},
	{
		key: "mmx",
		title: "MMX 技能维护",
		eyebrow: "技能补强",
		description: "盘点并维护与 mmx 相关的技能和用法。",
		prompt: "列出与 mmx 相关的 skill 并说明用途",
	},
	{
		key: "report",
		title: "调研报告",
		eyebrow: "深度调研",
		description: "让 Agent 先做结构化调研，再给出结果摘要。",
		prompt: "调研当前仓库前端改造成单工作台的可行方案，并给出分步建议",
	},
];

const expertDeck = [
	{
		key: "skill-router",
		title: "Skill Router",
		kicker: "技能编排",
		description: "围绕 skills 做盘点、维护和补充，适合整理运行时知识库。",
		previewTitle: "知识库整理",
		previewMeta: "skills / autoskill / 路由",
		themeClass: "is-skill",
		prompt: "分析当前 workspace 中最值得整理的 skills，并给出维护建议",
	},
	{
		key: "frontend-builder",
		title: "网页制作",
		kicker: "前端执行",
		description: "用本地 Agent 驱动前端改造、页面重构和样式优化。",
		previewTitle: "页面落地",
		previewMeta: "UI / 结构 / 样式",
		themeClass: "is-frontend",
		prompt: "根据当前仓库前端结构，生成下一步 UI 改造任务清单",
	},
	{
		key: "researcher",
		title: "调研报告",
		kicker: "问题分析",
		description: "用于代码调研、站点分析和方案对比，适合重任务入口。",
		previewTitle: "方案比较",
		previewMeta: "调研 / 结论 / 优先级",
		themeClass: "is-research",
		prompt: "总结当前项目最关键的运行时能力，并按优先级分类",
	},
];

function summarizeResult(result: CliRunResponse | null) {
	if (!result) {
		return "";
	}
	const source = result.stdout.trim() || result.stderr.trim();
	if (!source) {
		return "命令已返回，但没有输出内容。";
	}
	return source
		.split(/\n+/)
		.filter(Boolean)
		.slice(0, 3)
		.join(" ")
		.slice(0, 220);
}

function persistPromptHistory(item: PromptHistoryItem, limit = 12) {
	const nextHistory = [item, ...readPromptHistory()].slice(0, limit);
	if (typeof window !== "undefined") {
		window.localStorage.setItem(historyStorageKey, JSON.stringify(nextHistory));
		emitHistoryUpdated(nextHistory);
	}
	return nextHistory;
}

export function AgentPage({ requestedSection = "composer", onSectionActive }: AgentPageProps) {
	const [prompt, setPrompt] = useState("");
	const [workspace, setWorkspace] = useState("");
	const [mcp, setMcp] = useState(false);
	const [advancedKeys, setAdvancedKeys] = useState<string[]>([]);
	const [result, setResult] = useState<CliRunResponse | null>(null);
	const [loading, setLoading] = useState(false);
	const [error, setError] = useState<string>();
	const [history, setHistory] = useState<PromptHistoryItem[]>([]);
	const {
		health,
		statusResult,
		loading: statusLoading,
		error: statusError,
		refreshWorkspace,
	} = useWorkspaceStatus();

	useEffect(() => {
		setHistory(readPromptHistory());
		const pendingPrompt = consumeRequestedPrompt();
		if (pendingPrompt) {
			setPrompt(pendingPrompt);
		}
	}, []);

	useEffect(() => {
		function handleHistoryUpdated(event: Event) {
			const detail = (event as CustomEvent<HistoryUpdatedDetail>).detail;
			setHistory(detail?.history ?? readPromptHistory());
		}

		window.addEventListener(historyUpdatedEvent, handleHistoryUpdated as EventListener);
		return () => {
			window.removeEventListener(historyUpdatedEvent, handleHistoryUpdated as EventListener);
		};
	}, []);

	useEffect(() => {
		function handlePromptRequested(event: Event) {
			const detail = (event as CustomEvent<PromptRequestedDetail>).detail;
			if (detail?.prompt) {
				setPrompt(detail.prompt);
				clearRequestedPrompt();
			}
		}

		window.addEventListener(promptRequestedEvent, handlePromptRequested as EventListener);
		return () => {
			window.removeEventListener(promptRequestedEvent, handlePromptRequested as EventListener);
		};
	}, []);

	useEffect(() => {
		const frameId = window.requestAnimationFrame(() => {
			document.getElementById(requestedSection)?.scrollIntoView({ behavior: "smooth", block: "start" });
			onSectionActive?.(requestedSection);
		});

		return () => {
			window.cancelAnimationFrame(frameId);
		};
	}, [onSectionActive, requestedSection]);

	useEffect(() => {
		const sections = observedSections
			.map(sectionId => document.getElementById(sectionId))
			.filter((element): element is HTMLElement => element !== null);
		if (sections.length === 0) {
			return;
		}

		const observer = new IntersectionObserver(
			entries => {
				const visibleEntries = entries.filter(entry => entry.isIntersecting);
				if (visibleEntries.length === 0) {
					return;
				}

				const nextSectionId = visibleEntries.sort((left, right) => right.intersectionRatio - left.intersectionRatio)[0]?.target.id;
				if (nextSectionId && isAgentSectionKey(nextSectionId)) {
					onSectionActive?.(nextSectionId);
				}
			},
			{
				threshold: [0.2, 0.35, 0.5, 0.65],
				rootMargin: "-20% 0px -55% 0px",
			},
		);

		sections.forEach(section => observer.observe(section));
		return () => {
			observer.disconnect();
		};
	}, [onSectionActive]);

	const historyData = useMemo(() => history.slice(0, 6), [history]);
	const successCount = useMemo(() => history.filter(item => item.exitCode === 0).length, [history]);
	const resultSummary = useMemo(() => summarizeResult(result), [result]);
	const statusSummary = useMemo(() => summarizeResult(statusResult), [statusResult]);

	function moveToComposer(nextPrompt: string) {
		setPrompt(nextPrompt);
		jumpToSection("composer");
	}

	function jumpToSection(sectionId: AgentSectionKey) {
		onSectionActive?.(sectionId);
		document.getElementById(sectionId)?.scrollIntoView({ behavior: "smooth", block: "start" });
	}

	function toggleAdvanced() {
		setAdvancedKeys(current => (current.length > 0 ? [] : ["advanced"]));
	}

	function handlePromptKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
		if (event.nativeEvent.isComposing) {
			return;
		}
		if (event.key !== "Enter" || (!event.metaKey && !event.ctrlKey)) {
			return;
		}
		event.preventDefault();
		if (loading) {
			return;
		}
		void executePrompt();
	}

	async function executePrompt(nextPrompt?: string) {
		if (loading) {
			return;
		}
		const resolvedPrompt = (nextPrompt ?? prompt).trim();
		if (!resolvedPrompt) {
			setError("请输入要执行的任务。");
			return;
		}
		setLoading(true);
		setError(undefined);
		try {
			const payload = await runCli({
				command: "agent",
				prompt: resolvedPrompt,
				workspace: workspace.trim() || undefined,
				mcp,
			});
			setResult(payload);
			const nextHistory = persistPromptHistory({
				title: resolvedPrompt,
				prompt: resolvedPrompt,
				source: "agent",
				runAt: payload.runAt,
				exitCode: payload.exitCode,
				durationMs: payload.durationMs,
				commandLine: payload.commandLine,
			});
			startTransition(() => {
				setHistory(nextHistory);
			});
		} catch (runError) {
			setError(runError instanceof Error ? runError.message : String(runError));
		} finally {
			setLoading(false);
		}
	}

	return (
		<div className="page-stack workbench-page">
			<section id="composer" className="workbench-section">
				<Card className="panel-card workbench-hero-card">
					<div className="workbench-hero-grid">
						<div className="workbench-hero-copy">
							<Tag color="processing">MiniClaw Agent Workspace</Tag>
							<Typography.Paragraph type="secondary" className="workbench-lead">
								输入目标、选择能力，然后直接执行本地 CLI 任务。
							</Typography.Paragraph>
							<div className="workbench-chip-row">
								<span className="workbench-pill">本地 CLI</span>
								<span className="workbench-pill">技能路由</span>
								<span className="workbench-pill">运行记录</span>
							</div>
						</div>
						<div className="workbench-glance-grid">
							<div className="workbench-glance-card">
								<span className="workbench-glance-label">CLI 模式</span>
								<strong>{health?.binaryMode === "binary" ? "已构建二进制" : "go run 回退"}</strong>
							</div>
							<div className="workbench-glance-card">
								<span className="workbench-glance-label">服务端口</span>
								<strong>{health?.port ?? 5020}</strong>
							</div>
							<div className="workbench-glance-card">
								<span className="workbench-glance-label">累计运行</span>
								<strong>{history.length}</strong>
							</div>
						</div>
					</div>

						<div className="workbench-live-grid">
							<div className="workbench-live-primary">
								<div className="workbench-composer-shell">
									<Input.TextArea
										value={prompt}
										onChange={event => setPrompt(event.target.value)}
										onKeyDown={handlePromptKeyDown}
										autoSize={{ minRows: 6, maxRows: 14 }}
										showCount
										maxLength={4000}
										className="workbench-task-input"
										placeholder="例如：总结当前运行时状态，并给出下一步建议"
										aria-keyshortcuts="Meta+Enter Control+Enter"
									/>
									<div className="workbench-composer-footer">
										<div className="workbench-toolbar-meta workbench-toolbar-tools">
											<Button type="text" size="small" className="workbench-toolbar-button" onClick={() => jumpToSection("scenes")}>
												技能
											</Button>
											<Button type="text" size="small" className="workbench-toolbar-button" onClick={() => jumpToSection("history")}>
												运行记录
											</Button>
											<Button type="text" size="small" className="workbench-toolbar-button" onClick={toggleAdvanced}>
												高级能力
											</Button>
										</div>
										<div className="workbench-toolbar-actions">
											<span className="workbench-inline-chip">Local CLI</span>
											<span className="workbench-inline-chip">Cmd/Ctrl + Enter</span>
											<Button
												type="primary"
												loading={loading}
												onClick={() => void executePrompt()}
												aria-keyshortcuts="Meta+Enter Control+Enter"
												title="Cmd/Ctrl + Enter"
											>
												发送任务
											</Button>
										</div>
									</div>
								</div>

								<Collapse
									className="workbench-advanced"
									activeKey={advancedKeys}
									onChange={keys => setAdvancedKeys((Array.isArray(keys) ? keys : [keys]).map(String))}
									items={[
										{
											key: "advanced",
											label: "高级设置",
											children: (
												<div className="workbench-advanced-body">
													<div className="workbench-advanced-field">
														<Typography.Text strong>Workspace 覆盖</Typography.Text>
														<Input
															value={workspace}
															onChange={event => setWorkspace(event.target.value)}
															placeholder="例如：/Users/byf/.miniclaw/workspace"
														/>
													</div>
													<div className="workbench-advanced-field workbench-advanced-field-compact">
														<Typography.Text strong>执行能力</Typography.Text>
														<label className="workbench-switch-row">
															<span>附带 --mcp</span>
															<Switch checked={mcp} onChange={setMcp} />
														</label>
														<div className="workbench-advanced-actions">
															<Button onClick={() => setPrompt("")}>清空输入</Button>
															<Button onClick={() => void refreshWorkspace()} loading={statusLoading}>
																刷新工作区状态
															</Button>
														</div>
													</div>
												</div>
											),
										},
									]}
								/>
							</div>

							<div id="composer-result" className="workbench-live-result">
								<RunResultPanel
									title="最新任务输出"
									loading={loading}
									error={error}
									result={result}
									emptyText="发送任务后，最新的 stdout 和 stderr 会固定显示在这里。"
								/>
							</div>
						</div>

					<div className="scene-chip-rail" aria-label="任务场景">
						{sceneTemplates.map(scene => (
							<button key={scene.key} type="button" className="scene-chip" onClick={() => moveToComposer(scene.prompt)}>
								<span className="scene-chip-title">{scene.title}</span>
								<span className="scene-chip-meta">{scene.description}</span>
							</button>
						))}
					</div>

					{error ? <Alert className="agent-inline-alert" type="error" showIcon message={error} /> : null}
				</Card>
			</section>

			<section id="scenes" className="workbench-section">
				<Card className="panel-card workbench-card workbench-expert-gallery" title="专家套组">
					<div className="expert-gallery-grid">
						{expertDeck.map(expert => (
							<button key={expert.key} type="button" className={`expert-gallery-card ${expert.themeClass}`} onClick={() => moveToComposer(expert.prompt)}>
								<div className="expert-gallery-art" />
								<div className="expert-gallery-copy">
									<span className="expert-gallery-kicker">{expert.kicker}</span>
									<Typography.Title level={4}>{expert.title}</Typography.Title>
									<Typography.Paragraph type="secondary">{expert.description}</Typography.Paragraph>
									<div className="expert-gallery-meta">
										<span>{expert.previewTitle}</span>
										<span>{expert.previewMeta}</span>
									</div>
									<span className="expert-gallery-cta">以此开题</span>
								</div>
							</button>
						))}
					</div>
				</Card>
			</section>

			<section id="history" className="workbench-section">
				<Row gutter={[16, 16]}>
					<Col xs={24} xl={10}>
						<Card className="panel-card workbench-card" title="运行记录">
							{historyData.length === 0 ? (
								<Empty description="还没有执行历史。" />
							) : (
								<div className="history-list">
									{historyData.map(item => (
										<div key={`${item.runAt}-${item.prompt}`} className="history-item">
											<div className="history-item-copy">
												<Typography.Text strong>{item.title}</Typography.Text>
												<Typography.Paragraph type="secondary">
													{item.source === "command" ? "快捷命令" : "Agent 任务"} · {new Date(item.runAt).toLocaleString()} · {item.durationMs} ms
												</Typography.Paragraph>
											</div>
											<div className="history-item-actions">
												<Tag color={item.exitCode === 0 ? "success" : "error"}>{item.exitCode === 0 ? "成功" : "失败"}</Tag>
												<Button type="link" onClick={() => moveToComposer(item.prompt)}>
													复用
												</Button>
											</div>
										</div>
									))}
								</div>
							)}
						</Card>
					</Col>
					<Col xs={24} xl={14}>
						<Card className="panel-card workbench-card" title="最近一次任务">
							{result ? (
								<div className="task-summary-card">
									<div className="task-summary-head">
										<Tag color={result.exitCode === 0 ? "success" : "error"}>{result.exitCode === 0 ? "任务完成" : "任务失败"}</Tag>
										<Typography.Text type="secondary">{new Date(result.runAt).toLocaleString()}</Typography.Text>
									</div>
									<Typography.Title level={4}>{result.commandLine}</Typography.Title>
									<Typography.Paragraph>{resultSummary}</Typography.Paragraph>
									<Descriptions column={1} size="small">
										<Descriptions.Item label="耗时">{result.durationMs} ms</Descriptions.Item>
										<Descriptions.Item label="CLI 模式">{result.binaryMode}</Descriptions.Item>
									</Descriptions>
									<div className="task-summary-actions">
										<Button onClick={() => jumpToSection("composer")}>继续编辑</Button>
										<Button type="link" onClick={() => jumpToSection("composer")}>
											查看完整输出
										</Button>
									</div>
								</div>
							) : (
								<Empty description="提交任务后，这里会显示结果摘要。" />
							)}
						</Card>
					</Col>
				</Row>
			</section>

			<section id="workspace" className="workbench-section">
				<Row gutter={[16, 16]}>
					<Col xs={24} md={8}>
						<Card className="panel-card workbench-card">
							<Statistic title="累计成功任务" value={successCount} loading={loading && history.length === 0} />
						</Card>
					</Col>
					<Col xs={24} md={8}>
						<Card className="panel-card workbench-card">
							<Statistic title="服务时间" value={health?.serverTime ? new Date(health.serverTime).toLocaleTimeString() : "--"} loading={statusLoading} />
						</Card>
					</Col>
					<Col xs={24} md={8}>
						<Card className="panel-card workbench-card">
							<Statistic title="仓库" value={health?.repoRoot.split("/").filter(Boolean).at(-1) || "--"} loading={statusLoading} />
						</Card>
					</Col>
				</Row>

				<Card className="panel-card workbench-card" title="工作区状态">
					<div className="workspace-status-head">
						<div>
							<Typography.Text strong>状态摘要</Typography.Text>
							<Typography.Paragraph type="secondary">{statusSummary || "状态加载完成后，会在这里给出摘要。"}</Typography.Paragraph>
						</div>
						<Button onClick={() => void refreshWorkspace()} loading={statusLoading}>
							刷新
						</Button>
					</div>
					{statusError ? <Alert type="error" showIcon message={statusError} /> : null}
				</Card>

				<Collapse
					className="workspace-output-collapse"
					items={[
						{
							key: "workspace-output",
							label: "查看原始状态输出",
							children: (
								<RunResultPanel
									title="miniclaw status"
									loading={statusLoading}
									error={statusError}
									result={statusResult}
									emptyText="工作区状态加载后，这里会显示原始回包。"
								/>
							),
						},
					]}
				/>
			</section>
		</div>
	);
}