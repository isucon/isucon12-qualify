import { createRouter, createWebHistory, RouteRecordRaw } from 'vue-router'
import LandingPageView from '@/views/tenant/player/LandingPageView.vue'
import MyPageView from '@/views/tenant/player/MyPageView.vue'
import SingleCompetitionView from '@/views/tenant/player/SingleCompetitionView.vue'
import SinglePlayerView from '@/views/tenant/player/SinglePlayerView.vue'
import OrganizerMainView from '@/views/tenant/organizer/OrganizerMainView.vue'
import PlayerListView from '@/views/tenant/organizer/PlayerListView.vue'
import CompetitionListView from '@/views/tenant/organizer/CompetitionListView.vue'
import TenantBillingView from '@/views/tenant/organizer/TenantBillingView.vue'

const routes: Array<RouteRecordRaw> = [
  {
    path: "/",
    name: "lp",
    component: LandingPageView,
  },
  {
    path: "/mypage",
    name: "mypage",
    component: MyPageView,
  },
  {
    path: "/competition/:competition_id",
    name: "competition",
    component: SingleCompetitionView,
  },
  {
    path: "/player/:player_id",
    name: "player",
    component: SinglePlayerView,
  },
  {
    path: "/organizer",
    name: "organizer",
    component: OrganizerMainView,
  },
  {
    path: "/organizer/players",
    name: "players",
    component: PlayerListView,
  },
  {
    path: "/organizer/competitions",
    name: "competitions",
    component: CompetitionListView,
  },
  {
    path: "/organizer/billing",
    name: "billing",
    component: TenantBillingView,
  },
  {
    path: '/:catchall(.*)',
    name: 'notfound',
    redirect: '/',
  },
];

const router = createRouter({
  history: createWebHistory(process.env.BASE_URL),
  routes,
});

export default router;
