/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
export type CodexRadarWindow = {
  open?: boolean
  status?: string
  action?: string
  message?: string
  title?: string
  scope?: string
  opened_at?: string | null
  closed_at?: string | null
  source_url?: string
}

export type CodexRadarPrediction = {
  level?: string
  probability_24h?: number
  probability_48h?: number
  summary?: string
  summary_en?: string
  updated_at?: string
}

export type CodexRadarTiboPresence = {
  timezone?: string
  location_label_zh?: string
  location_label_en?: string
  probability?: number
  confidence?: string
  evidence_summary_zh?: string
  evidence_summary_en?: string
  source_urls?: string[]
  should_display?: boolean
  safety_note_zh?: string
  safety_note_en?: string
  updated_at?: string
  observed_at?: string
  observations_considered?: number
}

export type CodexRadarModelIqLatest = {
  date?: string
  score?: number
  status?: string
  passed?: number
  tasks?: number
  invalid?: number
  total_tokens?: number
  input_tokens?: number
  cached_input_tokens?: number
  output_tokens?: number
  wall_seconds?: number
  wall_time_human?: string
  model?: string
  reasoning_effort?: string
  valid_tasks?: number
  cost_usd?: number
}

export type CodexRadarQuotaRadar = {
  date?: string
  updated_at?: string
  cost_usd?: number
  total_tokens?: number
  basis_window_label?: string
  base_cost_usd?: number
  adjusted_delta?: number
  raw_delta?: number
  rate?: number
  trend?: string
}

export type CodexRadarQuotaCheck = {
  status?: string
  checked_at?: string
  plan_type?: string
  rate_limit_reset_credits_available_count?: number
  limit_reached?: boolean
  allowed?: boolean
}

export type CodexRadarPublicSummary = {
  schema_version?: string
  service?: string
  type?: string
  monitored_at?: string
  timezone?: string
  window_open?: boolean
  status?: string
  recommended_action?: string
  window?: CodexRadarWindow
  prediction?: CodexRadarPrediction
  tibo_presence?: CodexRadarTiboPresence
  links?: {
    html?: string
    rss?: string
    full_api?: string
  }
  api_access?: {
    requirements?: {
      attribution_required?: boolean
      attribution_text?: string
      site?: string
    }
  }
  model_iq?: {
    latest?: CodexRadarModelIqLatest
    recent_days?: CodexRadarModelIqLatest[]
    quota_radar?: CodexRadarQuotaRadar
    quota_check?: CodexRadarQuotaCheck
  }
}
