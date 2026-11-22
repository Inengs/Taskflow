import api from "../axios/api";
import type { Project, ProjectDetail, ProjectInput, ProjectListItem, ProjectMember } from "../types/project";
import type { ProjectTask, CreateTaskInput } from "../types/task";


export async function getProjects(): Promise<{ message: string; projects: ProjectListItem[] }> {
  // Trailing slash on purpose: the route is registered as "/", and without it
  // Gin 301-redirects, which drops the Authorization header cross-origin.
  const { data } = await api.get("/projects/");
  return data;
}

export async function createProject(input: ProjectInput): Promise<{ message: string; project: Project }> {
  const { data } = await api.post("/projects/", input);
  return data;
}

export async function getProject(id: number): Promise<{ message: string; project: ProjectDetail }> {
  const { data } = await api.get(`/projects/${id}`); // no trailing slash on /:id routes
  return data;
}

export async function getProjectTasks(
  projectId: number
): Promise<{ message: string; tasks: ProjectTask[]; truncated: boolean }> {
  const { data } = await api.get(`/projects/${projectId}/tasks`);
  return data;
}

export async function getProjectMembers(
  projectId: number
): Promise<{ message: string; members: ProjectMember[] }> {
  const { data } = await api.get(`/projects/${projectId}/members`);
  return data;
}

export async function createProjectTask(
  projectId: number,
  input: CreateTaskInput
): Promise<{ message: string; task: ProjectTask }> {
  const { data } = await api.post(`/projects/${projectId}/tasks`, input);
  return data;
}