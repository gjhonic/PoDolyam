import type { Friend, Participant } from './types'

export function organizerParticipant(name: string): Participant {
  const normalized = name.trim()
  if (!normalized) throw new Error('Имя организатора не заполнено')
  return { id: 'me', name: normalized, order: 0 }
}

export function participantFromFriend(participants: Participant[], friend: Friend): Participant | null {
  if (participants.some(person => person.id === friend.id)) return null
  const order = Math.max(-1, ...participants.map(person => person.order)) + 1
  return { id: friend.id, name: friend.name, order }
}
