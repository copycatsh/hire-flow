export interface ServiceStats {
  total: number;
  error?: string;
}

export interface DashboardStats {
  jobs: ServiceStats;
  contracts: ServiceStats;
  wallets: ServiceStats;
}
