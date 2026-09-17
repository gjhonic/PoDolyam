import { describe, expect, it } from 'vitest'
import { organizerParticipant, participantFromFriend } from './participants'

const dima = { id: 'friend-dima', name: 'Дима', phone: '+7 900', birthday: '1990-09-17' }

describe('участники из профиля и справочника', () => {
  it('создаёт организатора с постоянным ID плательщика', () => {
    expect(organizerParticipant(' Женя ')).toEqual({ id: 'me', name: 'Женя', order: 0 })
  })

  it('сохраняет ID друга и ставит его после текущих участников', () => {
    expect(participantFromFriend([{ id: 'me', name: 'Женя', order: 0 }], dima)).toEqual({ id: 'friend-dima', name: 'Дима', order: 1 })
  })

  it('не добавляет одного друга второй раз', () => {
    expect(participantFromFriend([{ id: 'friend-dima', name: 'Дима', order: 4 }], dima)).toBeNull()
  })
})
