import type { InjectionKey, Ref } from 'vue'
import { inject } from 'vue'

import type { UserProfile } from '@/renderer/types'

export interface UserProfileContext {
  profile: Readonly<Ref<UserProfile | null>>
  replaceProfile: (next: UserProfile) => void
}

export const userProfileKey: InjectionKey<UserProfileContext> = Symbol('user-profile')

export function useUserProfile(): UserProfileContext {
  const context = inject(userProfileKey)
  if (!context)
    throw new Error('useUserProfile() 必须在 App 用户资料上下文中使用')
  return context
}

export function useOptionalUserProfile(): UserProfileContext | undefined {
  return inject(userProfileKey, undefined)
}
