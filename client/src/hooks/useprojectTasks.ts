import { useCallback, useEffect, useRef, useState } from "react";
import { getProjectTasks } from "../api/projects";
import type { ProjectTask } from "../types/task";

interface State {
  tasks: ProjectTask[];
  truncated: boolean; // the server capped the list at 500
  loading: boolean;
  error: string | null;
}

export function useProjectTasks(projectId: number) {
  const [state, setState] = useState<State>({
    tasks: [],
    truncated: false,
    loading: true,
    error: null,
  });
  // Ignore responses from an older request (e.g. after switching projects quickly).
  const latest = useRef(0);

  const refetch = useCallback(async () => {
    const requestId = ++latest.current;
    setState((s) => ({ ...s, loading: true, error: null }));
    try {
      const { tasks, truncated } = await getProjectTasks(projectId);
      if (requestId !== latest.current) return;
      setState({ tasks, truncated, loading: false, error: null });
    } catch (err) {
      if (requestId !== latest.current) return;
      setState({
        tasks: [],
        truncated: false,
        loading: false,
        error: err instanceof Error ? err.message : "Failed to load tasks",
      });
    }
  }, [projectId]);

  useEffect(() => {
    refetch();
  }, [refetch]);

  return { ...state, refetch };
}