export interface MediaPlaybackPort {
  stopMedia(root: ParentNode | undefined): void;
}

export interface ChoiceOption<Value extends string> {
  value: Value;
  label: string;
}
