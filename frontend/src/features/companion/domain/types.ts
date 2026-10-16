export interface WindowPort {
  ShowMain(): Promise<void>;
  HideMain(): Promise<void>;
  ResetWidget(): Promise<void>;
  Quit(): Promise<void>;
  ShowReviews(): Promise<void>;
  RefreshReminders(): Promise<void>;
  ToggleFloatingReview(): Promise<void>;
  HideFloatingReview(): Promise<void>;
}
