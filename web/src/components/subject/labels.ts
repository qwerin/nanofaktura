import type { SubjectType } from '@/api/types'

export const subjectTypeLabels: Record<SubjectType, string> = {
  customer: 'Odběratel',
  supplier: 'Dodavatel',
  both: 'Odběratel i dodavatel',
}

export const subjectTypeOptions = (Object.keys(subjectTypeLabels) as SubjectType[]).map((value) => ({
  value,
  label: subjectTypeLabels[value],
}))
