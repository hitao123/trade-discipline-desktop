import { createMemoryHistory, createRouter } from 'vue-router'

import ExecutionsView from './views/ExecutionsView.vue'
import MarketView from './views/MarketView.vue'
import PlansView from './views/PlansView.vue'
import PositionsView from './views/PositionsView.vue'
import SettingsView from './views/SettingsView.vue'
import TodayView from './views/TodayView.vue'
import WatchlistView from './views/WatchlistView.vue'
import WeeklyReviewView from './views/WeeklyReviewView.vue'

export function createAppRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', name: 'today', component: TodayView },
      { path: '/watchlist', name: 'watchlist', component: WatchlistView },
      { path: '/plans', name: 'plans', component: PlansView },
      { path: '/executions', name: 'executions', component: ExecutionsView },
      { path: '/positions', name: 'positions', component: PositionsView },
      { path: '/market', name: 'market', component: MarketView },
      { path: '/reviews', name: 'reviews', component: WeeklyReviewView },
      { path: '/settings', name: 'settings', component: SettingsView },
    ],
  })
}

export const router = createAppRouter()
