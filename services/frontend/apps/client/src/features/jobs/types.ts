export interface Job {
  id: string;
  title: string;
  description: string;
  budget_min: number;
  budget_max: number;
  status: "draft" | "open" | "in_progress" | "closed";
  client_id: string;
  created_at: string;
  updated_at: string;
}

export interface CreateJobRequest {
  title: string;
  description: string;
  budget_min: number;
  budget_max: number;
  client_id: string;
}

export interface ListResponse<T> {
  items: T[];
  total: number;
}
