import { api } from './client'
import type { ApiEnvelope } from '@/types/api'
import type { EvidenceInquiry } from '@/types/evidence-inquiry'

export interface EvidenceInquiryQuery {
  proposal_id?: number
  status?: 'pending' | 'answered'
  page?: number
  page_size?: number
}

export interface EvidenceInquiryCreateInput {
  question: string
  observation_ids: number[]
}

export interface EvidenceInquiryAnswerInput {
  response: string
  version: number
}

export const evidenceInquiryApi = {
  list: (params?: EvidenceInquiryQuery) => api.get<ApiEnvelope<EvidenceInquiry[]>>('/evidence-inquiries', { params }),
  detail: (id: number) => api.get<ApiEnvelope<EvidenceInquiry>>(`/evidence-inquiries/${id}`),
  create: (proposalID: number, body: EvidenceInquiryCreateInput, idempotencyKey: string) =>
    api.post<ApiEnvelope<EvidenceInquiry>>(`/proposals/${proposalID}/evidence-inquiries`, body, {
      headers: { 'Idempotency-Key': idempotencyKey },
    }),
  answer: (id: number, body: EvidenceInquiryAnswerInput) =>
    api.post<ApiEnvelope<EvidenceInquiry>>(`/evidence-inquiries/${id}/answer`, body),
}
