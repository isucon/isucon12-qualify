<template>
  <div class="single-player">
    <h2>プレイヤー情報</h2>
    <template v-if="player">
      <ul >
        <li>プレイヤーID: {{ player.id }}</li>
        <li>プレイヤー名: {{ player.display_name }}</li>
        <li>参加資格情報: {{ player.is_disqualified ? '失格' : '有効' }}</li>
      </ul>

      <h3>登録されたスコア</h3>
      <p>TBD...</p>

    </template>
  </div>
</template>

<script lang="ts">
import { ref, onMounted, defineComponent } from 'vue'
import { useRoute } from 'vue-router'

import axios from 'axios'

type Player = {
  display_name: string
  id: string
  is_disqualified: boolean
}

type PlayerScore = {
  competition_title: string
  score: number
}

export default defineComponent({
  setup() {
    const route = useRoute()
    const playerId = route.params.player_id

    const player = ref<Player>()
    const scores = ref<PlayerScore[]>([])
    const fetchPlayer = async () => {
      const res = await axios.get(`/api/player/player/${playerId}`)
      console.log(res.data)

      player.value = res.data.data.player
      scores.value = res.data.data.scores
    }

    onMounted(() => {
      fetchPlayer()
    })

    return {
      player,
      scores,
    }
  },
})
</script>
