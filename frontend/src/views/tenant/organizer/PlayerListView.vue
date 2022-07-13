<template>
  <div class="players">
    <h2>
      プレイヤー一覧
    </h2>

    <TableBase
      :header="tableHeader"
      :data="tableData"
      :row-attr="players"
    >
      <template #cell-action="slotProps">
        <button
          v-if="!slotProps.row.is_disqualified"
          class="slim"
          :value="slotProps.row.id"
          @click="handleDisqualify"
        >
          失格にする
        </button>
      </template>
    </TableBase>
  </div>
</template>

<script lang="ts">
import { ref, computed, onMounted, defineComponent } from 'vue'
import axios from 'axios'

import TableBase, { TableColumn } from '@/components/parts/TableBase.vue'

type Player = {
  id: string
  display_name: string
  is_disqualified: boolean
}

export default defineComponent({
  name: 'PlayerListView',
  components: {
    TableBase,
  },
  setup() {
    const players = ref<Player[]>([])

    const fetchPlayers = async () => {
      const res = await axios.get('/api/organizer/players')
      if (!res.data.status) {
        window.alert('failed to fetch players: status=false')
        return
      }

      players.value = res.data.data.players
    }

    onMounted(() => {
      fetchPlayers()
    })

    const handleDisqualify = (evt: MouseEvent) => {
      const target = evt.target as HTMLButtonElement
      if (!target) return

      const playerId = target.value
      disqualify(playerId)
    }

    const disqualify = async (playerId: string) => {
      const res = await axios.post(`/api/organizer/player/${playerId}/disqualified`)
      if (!res || !res.data.status) {
        window.alert('faied to finish')
        return
      }
      fetchPlayers() // 画面を更新!
    }

          // <th class="player-id">プレイヤーID</th>
          // <th class="player-name">プレイヤー名</th>
          // <th class="player-is-disqualified">参加資格</th>
          // <th class="action"></th>

    const tableHeader: TableColumn[] = [
      {
        width: '15%',
        align: 'center',
        text: 'プレイヤーID',
      },
      {
        width: '55%',
        align: 'left',
        text: 'プレイヤー名',
      },
      {
        width: '15%',
        align: 'center',
        text: '参加資格',
      },
      {
        width: '15%',
        align: 'center',
        text: '',
      },
    ]

    const tableData = computed<string[][]>(() => 
      players.value.map(p => {
        return [
          p.id,
          p.display_name,
          p.is_disqualified ? '失格' : '',
          '##slot##cell-action',
        ]
      })
    )

    return {
      players,
      handleDisqualify,
      tableHeader,
      tableData,
    }
  },
})
</script>

<style scoped>
.players {
  padding: 0 20px 20px;
}

.player-list {
  border: 1px solid lightgray;
  border-collapse: collapse;
  width: 100%;
}

th, td {
  padding: 4px;
  border: 1px solid gray;
}

.player-id,
.player-is-disqualified {
  width: 15%;
  text-align: center;
}

.action {
  width: 20%;
  text-align: center;
}

.player-name {
  width: 50%;
}

</style>