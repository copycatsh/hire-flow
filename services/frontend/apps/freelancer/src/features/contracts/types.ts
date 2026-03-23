export interface ListResponse<T> {
  items: T[];
  total: number;
}

export interface Contract {
  id: string;
  client_id: string;
  freelancer_id: string;
  title: string;
  description: string;
  amount: number;
  currency: string;
  status: string;
  client_wallet_id: string;
  freelancer_wallet_id: string;
  hold_id?: string;
  created_at: string;
  updated_at: string;
}
