<template>
  <div class="single-player">
    <h2>プレイヤー情報</h2>
    <template v-if="isLoading">
      読み込み中…
    </template>
    <template v-if="player">
      <ul >
        <li>プレイヤーID: {{ player.id }}</li>
        <li>プレイヤー名: {{ player.display_name }}</li>
        <li>参加資格情報: {{ player.is_disqualified ? '失格' : '有効' }}</li>
      </ul>

      <h3>登録されたスコア</h3>
      <TableBase
        :header="tableHeader"
        :data="tableData"
      >
      </TableBase>
    </template>
  </div>
</template>

<script lang="ts">
import { ref, computed, onMounted, defineComponent } from 'vue'
import { useRoute } from 'vue-router'

import axios from 'axios'

import TableBase, { TableColumn } from '@/components/parts/TableBase.vue'

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
  components:{
    TableBase,
  },
  setup() {
    const route = useRoute()
    const playerId = route.params.player_id

    const isLoading = ref(true)
    const player = ref<Player>()
    const scores = ref<PlayerScore[]>([])
    const fetchPlayer = async () => {
      isLoading.value = true
      try {
        const res = await axios.get(`/api/player/player/${playerId}`)

        player.value = res.data.data.player
        scores.value = res.data.data.scores
      } catch (e) {
        window.alert('failed to fetch player info: ' + e)
      } finally {
        isLoading.value = false
      }
    }

    onMounted(() => {
      fetchPlayer()
    })

    const tableHeader: TableColumn[] = [
      {
        width: '80%',
        align: 'left',
        text: '大会名',
      },
      {
        width: '20%',
        align: 'right',
        text: 'スコア',
      },
    ]

    const tableData = computed<string[][]>(() => 
      scores.value.map(c => {
        return [
          c.competition_title,
          c.score.toLocaleString(),
        ]
      })
    )

    return {
      isLoading,
      player,
      scores,
      tableHeader,
      tableData,
    }
  },
})
</script>

<style scoped>
.single-player {
  padding: 0 20px 20px;
}
</style>