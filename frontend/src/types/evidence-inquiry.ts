export type EvidenceInquiryStatus = 'pending' | 'answered'

export interface EvidenceInquiryObservation {
  id: number
  observation_code: string
  point_geojson: string
  observed_at: string
  method: string
  horizontal_accuracy_m: number
  observation_state: string
  quality_note: string
}

export interface EvidenceInquiry {
  id: number
  inquiry_number: string
  proposal_id: number
  observation_ids: number[]
  observations: EvidenceInquiryObservation[]
  question: string
  response: string | null
  status: EvidenceInquiryStatus
  version: number
  created_by: number
  created_by_name: string
  responded_by?: number | null
  responded_by_name: string
  responded_at?: string | null
  created_at: string
  updated_at: string
}
