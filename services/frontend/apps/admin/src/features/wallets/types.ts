export interface Wallet {
  id: string;
  user_id: string;
  balance: number;
  currency: string;
  available_balance: number;
  created_at: string;
  updated_at: string;
}

export interface ListResponse<T> {
  items: T[];
  total: number;
}
