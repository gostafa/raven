export interface TransportConfiguration {
  preferences_storage_key: string;
  media_route_prefix: string;
  text_kind: string;
  schedules_changed: string;
  show_reviews: string;
  open_floating_review: string;
  close_floating_review: string;
}

export interface TransportFolder {
  id: string;
  title: string;
  description: string;
  tags: string[] | null;
}

export interface TransportCard {
  id: string;
  folder_id: string;
  items?: TransportItem[] | null;
  created_at: number;
  due_at: number;
  last_reviewed: number;
}

export interface TransportItem {
  id: string;
  folder_id: string;
  card_id: string;
  kind: string;
  text?: string;
  original_name?: string;
  mime_type?: string;
  relative_path?: string;
  byte_size?: number;
  reviews: { log: TransportReview; automatic: boolean }[] | null;
  schedule: TransportSchedule;
  last_notified_due: number;
  position: number;
}

export interface TransportSchedule {
  due: string;
  last_review: string;
  stability: number;
  difficulty: number;
  scheduled_days: number;
  reps: number;
  lapses: number;
  remaining_steps: number;
  state: number;
}

export interface TransportReview {
  due: string;
  review: string;
  scheduled_days: number;
  stability: number;
  difficulty: number;
  remaining_steps: number;
  rating: number;
  state: number;
}

export interface TransportDraft {
  id?: string;
  kind: string;
  text?: string;
  source_path?: string;
}

export interface TransportMedia {
  source_path: string;
  original_name: string;
  preview_url: string;
  byte_size: number;
}
