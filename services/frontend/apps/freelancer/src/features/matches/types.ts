export interface JobMatch {
  id: string;
  score: number;
}

export interface MatchResponse {
  matches: JobMatch[];
  total: number;
}
