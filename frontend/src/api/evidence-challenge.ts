import { api } from './client'
import type { ApiEnvelope } from '@/types/api'
import type { EvidenceChallenge } from '@/types/evidence-challenge'

export interface ChallengeQuery {
  state?: 'pending' | 'answered'
  page?: number
  page_size?: number
}

export interface ChallengeCreateInput {
  observation_ids: number[]
  question: string
}

export interface ChallengeResponseInput {
  response: string
}

export const evidenceChallengeApi = {
  list: (proposalId: number, params?: ChallengeQuery) =>
    api.get<ApiEnvelope<EvidenceChallenge[]>>(`/proposals/${proposalId}/challenges`, { params }),
  raise: (proposalId: number, body: ChallengeCreateInput, idempotencyKey: string) =>
    api.post<ApiEnvelope<EvidenceChallenge>>(`/proposals/${proposalId}/challenges`, body, {
      headers: { 'Idempotency-Key': idempotencyKey },
    }),
  respond: (challengeId: number, body: ChallengeResponseInput) =>
    api.post<ApiEnvelope<EvidenceChallenge>>(`/evidence-challenges/${challengeId}/respond`, body),
}
