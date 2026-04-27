import { Alert, Button, Card, Col, Row, Statistic, Table, Typography, message } from "antd";
import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";

import { fetchDashboardSummary, fetchHealth, type HealthResponse } from "../api";
import { formatDateTime } from "../format";
import { StatusTag } from "../components/StatusTag";
import type { DashboardSummary, RunRecord } from "../types";

export function DashboardPage() {
	const navigate = useNavigate();
	const [summary, setSummary] = useState<DashboardSummary | null>(null);
	const [health, setHealth] = useState<HealthResponse | null>(null);
	const [loading, setLoading] = useState(true);

	async function load() {
		setLoading(true);
		try {
			const [summaryPayload, healthPayload] = await Promise.all([fetchDashboardSummary(), fetchHealth()]);
			setSummary(summaryPayload);
			setHealth(healthPayload);
		} catch (error) {
			message.error(error instanceof Error ? error.message : String(error));
		} finally {
			setLoading(false);
		}
	}

	useEffect(() => {
		void load();
	}, []);

	return (
		<div className="page-section">
			<div className="page-actions">
				<div>
					<Typography.Title level={4} className="page-title">仪表盘</Typography.Title>
					<Typography.Text type="secondary" className="page-subtitle">
						聚合工作区 cron、sessions 和 skills，快速确认当前运行面和最近执行情况。
					</Typography.Text>
				</div>
				<Button onClick={() => void load()}>刷新</Button>
			</div>

			<Alert type="info" showIcon message="当前前端已接入真实工作区数据：任务和定时任务共用 cron 文件，执行记录来自 sessions，Skills 直接扫描 workspace 目录。" />

			<Row gutter={[16, 16]}>
				<Col xs={24} md={12} xl={6}>
					<Card loading={loading} size="small" variant="outlined">
						<Statistic title="任务总数" value={summary?.totalTasks ?? 0} />
					</Card>
				</Col>
				<Col xs={24} md={12} xl={6}>
					<Card loading={loading} size="small" variant="outlined">
						<Statistic title="Skills 总数" value={summary?.totalSkills ?? 0} />
					</Card>
				</Col>
				<Col xs={24} md={12} xl={6}>
					<Card loading={loading} size="small" variant="outlined">
						<Statistic title="待处理队列" value={summary?.pendingQueueCount ?? 0} />
					</Card>
				</Col>
				<Col xs={24} md={12} xl={6}>
					<Card loading={loading} size="small" variant="outlined">
						<Statistic title="今日成功 / 失败" value={`${summary?.successToday ?? 0} / ${summary?.failedToday ?? 0}`} />
					</Card>
				</Col>
			</Row>

			<Row gutter={[16, 16]}>
				<Col xs={24} xl={8}>
					<Card title="当前工作区" size="small" variant="outlined" loading={loading}>
						<div className="status-list">
							<div className="status-line"><span>通道</span><strong>{health?.gatewayChannel ?? "-"}</strong></div>
							<div className="status-line"><span>CLI 模式</span><strong>{health?.binaryMode === "binary" ? "已构建" : "go run"}</strong></div>
							<div className="status-line"><span>当前运行任务</span><strong>{summary?.currentRunId ?? "-"}</strong></div>
							<div className="status-line"><span>工作区目录</span><strong>{health?.workspaceDir ?? "-"}</strong></div>
						</div>
						<div className="button-row">
							<Button size="small" onClick={() => navigate("/tasks")}>查看任务</Button>
							<Button size="small" onClick={() => navigate("/skills")}>查看 Skills</Button>
						</div>
					</Card>
				</Col>
				<Col xs={24} xl={16}>
					<Card title="最近执行" size="small" variant="outlined">
						<Table<RunRecord>
							rowKey="id"
							size="small"
							pagination={false}
							dataSource={summary?.recentRuns ?? []}
							columns={[
								{ title: "执行项", dataIndex: "title" },
								{ title: "来源", dataIndex: "source", render: (value: string) => <StatusTag value={value} /> },
								{ title: "状态", dataIndex: "status", render: (value: string) => <StatusTag value={value} /> },
								{ title: "时间", dataIndex: "createdAt", render: (value: string) => formatDateTime(value) },
								{
									title: "操作",
									render: (_value, record) => <Button size="small" onClick={() => navigate(`/runs?focus=${record.id}`)}>查看</Button>,
								},
							]}
						/>
					</Card>
				</Col>
			</Row>
		</div>
	);
}