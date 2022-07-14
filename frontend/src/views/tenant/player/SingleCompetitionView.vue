<template>
  <div class="competition">
    <h2>
      大会情報
    </h2>

    <h3>スコアランキング</h3>
    <TableBase
      :header="tableHeader"
      :data="tableData"
      :row-attr="ranks"
    >
      <template #cell-playername="slotProps">
        <router-link :to=" `/player/${slotProps.row.player_id}`">{{ slotProps.row.player_display_name }}</router-link>
      </template>
      <template #footer>
        <tr v-if="!noMoreLoad">
          <td colspan="4" class="loading">
            <template v-if="isLoading">
              読み込み中... <span class="cycle-loop">め</span>
            </template>
            <template v-else>
              <button
                class="slim"
                @click="handleLoading"
              >
                さらに読み込む
              </button>
            </template>
          </td>
        </tr>
      </template>
    </TableBase>
  </div>
</template>

<script lang="ts">
import { ref, computed, onMounted, defineComponent } from 'vue'
import { useRoute } from 'vue-router'

import TableBase, { TableColumn } from '@/components/parts/TableBase.vue'

import { usePagerLoader } from '@/assets/hooks/usePagerLoader'

type Rank = {
  rank: number,
  score: number,
  player_id: string,
  player_display_name: string
}

export default defineComponent({
  components: {
    TableBase,
  },
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

    const tableHeader: TableColumn[] = [
      {
        width: '10%',
        align: 'center',
        text: '順位',
      },
      {
        width: '20%',
        align: 'center',
        text: 'プレイヤーID',
      },
      {
        width: '45%',
        align: 'left',
        text: 'プレイヤー名',
      },
      {
        width: '15%',
        align: 'right',
        text: 'スコア',
      },
    ]

    const tableData = computed<string[][]>(() => 
      ranks.value.map(r => {
        return [
          r.rank.toString(),
          r.player_id,
          '##slot##cell-playername',
          r.score.toLocaleString()
        ]
      })
    )

    return {
      ranks,
      isLoading,
      noMoreLoad,
      handleLoading,

      tableHeader,
      tableData,
    }
  },
})
</script>

<style scoped>
.competition {
  padding: 0 20px 20px;
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