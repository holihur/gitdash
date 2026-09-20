import { req } from "./core";

export interface FeedbackConfig {
  enabled: boolean;
}

export interface FeedbackResult {
  url: string;
  number: number;
}

export const feedbackApi = {
  feedbackConfig: () => req<FeedbackConfig>("/feedback"),
  submitFeedback: (input: { title?: string; body: string; url?: string }) =>
    req<FeedbackResult>("/feedback", {
      method: "POST",
      body: JSON.stringify(input),
    }),
};
