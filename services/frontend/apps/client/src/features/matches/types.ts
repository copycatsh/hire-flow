export interface MatchResult {
  id: string;
  score: number;
}

export interface MatchResponse {
  matches: MatchResult[];
  total: number;
}
