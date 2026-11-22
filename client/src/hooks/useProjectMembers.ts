import { useCallback, useEffect, useRef, useState } from "react";
import { getProjectMembers } from "../api/projects";
import type { ProjectMember } from "../types/project";

interface State {
  members: ProjectMember[];
  loading: boolean;
  error: string | null;
}

export function useProjectMembers(projectId: number) {
  const [state, setState] = useState<State>({ members: [], loading: true, error: null });
  const latest = useRef(0);

  const refetch = useCallback(async () => {
    const requestId = ++latest.current;
    setState((s) => ({ ...s, loading: true, error: null }));
    try {
      const { members } = await getProjectMembers(projectId);
      if (requestId !== latest.current) return;
      setState({ members, loading: false, error: null });
    } catch (err) {
      if (requestId !== latest.current) return;
      setState({
        members: [],
        loading: false,
        error: err instanceof Error ? err.message : "Failed to load members",
      });
    }
  }, [projectId]);

  useEffect(() => {
    refetch();
  }, [refetch]);

  return { ...state, refetch };
}