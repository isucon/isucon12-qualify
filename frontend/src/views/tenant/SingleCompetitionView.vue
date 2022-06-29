<template>
  <div class="competition">
    <h2>
      大会情報
    </h2>

    <h3>スコアランキング</h3>
    <table class="ranking">
      <thead>
        <tr>
          <th class="rank-number">順位</th>
          <th class="player-id">プレイヤーID</th>
          <th class="player-display-name">プレイヤー名</th>
          <th class="rank-score">スコア</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="r in ranks"
          :key="r.rank"
        >
          <td class="rank-number">{{ r.rank }}</td>
          <td class="player-id">{{ r.player_id }} <router-link :to="`/player/${r.player_id}`">→</router-link></td>
          <td class="player-display-name">{{ r.player_display_name }}</td>
          <td class="rank-score">{{ r.score }}</td>
        </tr>
        <tr v-if="!noMoreLoad">
          <td colspan="4" class="loading">
            <template v-if="isLoading">
              読み込み中... <span class="cycle-loop">め</span>
            </template>
            <template v-else>
            <button @click="handleLoading">さらに読み込む</button>
            </template>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script lang="ts">
import { ref, onMounted, defineComponent } from 'vue'
import { useRoute } from 'vue-router'

import { usePagerLoader } from '@/assets/hooks/usePagerLoader'

type Rank = {
  rank: number,
  score: number,
  player_id: string,
  player_display_name: string
}

export default defineComponent({
  setup() {
    const route = useRoute()
    const competitionId = route.params.competition_id

    const ranks = ref<Rank[]>([])

    const { fetch, isLoading, noMoreLoad } = usePagerLoader<Rank>({
      api: `/api/player/competition/${competitionId}/ranking`,
      payloadPath: 'ranks',
      pagerParam: 'rank_after',
      pagerKey: 'rank',
      limit: 100,
    })

    const fetchRanking = async () => {
      const ranking = await fetch()
      if (!ranking.length) {
        return
      }

      ranks.value.push(...ranking)
    }

    onMounted(() => {
      fetchRanking()
    })

    const handleLoading = () => {
      fetchRanking()
    }

    return {
      ranks,
      isLoading,
      noMoreLoad,
      handleLoading,
    }
  },
})
</script>

<style scoped>
.competition {
  padding: 0 20px 20px;
}


.ranking {
  border: 1px solid lightgray;
  border-collapse: collapse;
  width: 100%;
}

th, td {
  padding: 4px;
  border: 1px solid gray;
}

.rank-number {
  width: 10%;
  text-align: center;
}
.player-id {
  width: 20%;
  text-align: center;
}

.competition-title {
  width: 45%;
}

.rank-score {
  width: 15%;
  text-align: right;
}

th.rank-score{
  text-align: center;
}

@keyframes rotation {
  0%{ transform:rotate(0);}
  100%{ transform:rotate(360deg); }
}

.cycle-loop {
  display: inline-block;
  animation: 1s linear infinite rotation;
}

.loading {
  text-align: center;
  height: 40px;
}


</style>