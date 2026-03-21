export interface Contract {
  id: string;
  client_id: string;
  freelancer_id: string;
  title: string;
  description: string;
  amount: number;
  currency: string;
  status:
    | "PENDING"
    | "HOLD_PENDING"
    | "AWAITING_ACCEPT"
    | "ACTIVE"
    | "COMPLETING"
    | "COMPLETED"
    | "DECLINING"
    | "DECLINED"
    | "CANCELLED";
  client_wallet_id: string;
  freelancer_wallet_id: string;
  hold_id?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateContractRequest {
  client_id: string;
  freelancer_id: string;
  title: string;
  description: string;
  amount: number;
  currency: string;
  client_wallet_id: string;
  freelancer_wallet_id: string;
}
