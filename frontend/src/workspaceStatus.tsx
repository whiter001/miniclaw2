import { createContext, useContext, useEffect, useState, type ReactNode } from "react";

import { fetchHealth, runCli, type CliRunResponse, type HealthResponse } from "./api";

interface WorkspaceStatusContextValue {
	health: HealthResponse | null;
	statusResult: CliRunResponse | null;
	loading: boolean;
	error?: string;
	refreshWorkspace: () => void;
}

const WorkspaceStatusContext = createContext<WorkspaceStatusContextValue | null>(null);

export function WorkspaceStatusProvider({ children }: { children: ReactNode }) {
	const [health, setHealth] = useState<HealthResponse | null>(null);
	const [statusResult, setStatusResult] = useState<CliRunResponse | null>(null);
	const [loading, setLoading] = useState(true);
	const [error, setError] = useState<string>();
	const [refreshToken, setRefreshToken] = useState(0);

	useEffect(() => {
		let active = true;

		async function load() {
			setLoading(true);
			setError(undefined);
			try {
				const [healthPayload, statusPayload] = await Promise.all([fetchHealth(), runCli({ command: "status" })]);
				if (!active) {
					return;
				}
				setHealth(healthPayload);
				setStatusResult(statusPayload);
			} catch (loadError) {
				if (!active) {
					return;
				}
				setError(loadError instanceof Error ? loadError.message : String(loadError));
			} finally {
				if (active) {
					setLoading(false);
				}
			}
		}

		void load();
		return () => {
			active = false;
		};
	}, [refreshToken]);

	function refreshWorkspace() {
		setRefreshToken(current => current + 1);
	}

	return (
		<WorkspaceStatusContext.Provider value={{ health, statusResult, loading, error, refreshWorkspace }}>
			{children}
		</WorkspaceStatusContext.Provider>
	);
}

export function useWorkspaceStatus() {
	const context = useContext(WorkspaceStatusContext);
	if (!context) {
		throw new Error("useWorkspaceStatus must be used within WorkspaceStatusProvider.");
	}
	return context;
}