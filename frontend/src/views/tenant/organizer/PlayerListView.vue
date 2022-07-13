<template>
  <div class="players">
    <button class="add-players" @click="handleAddPlayers">
      プレイヤー追加
    </button>
    <AddPlayersModal
      v-show="showModal"
      @close="handleAddPlayersClose"
      @playersAdded="handlePlayersAdded"
    />
    
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
import AddPlayersModal from '@/components/tenant/AddPlayersModal.vue'

type Player = {
  id: string
  display_name: string
  is_disqualified: boolean
}

export default defineComponent({
  name: 'PlayerListView',
  components: {
    TableBase,
    AddPlayersModal,
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

    // AddPlayersModal関連
    const showModal = ref(false)
    const handleAddPlayers = () => {
      showModal.value = true
    }
    const handleAddPlayersClose = () => {
      showModal.value = false
    }
    const handlePlayersAdded = () => {
      fetchPlayers() // 画面を更新!
    }

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

      showModal,
      handleAddPlayers,
      handleAddPlayersClose,
      handlePlayersAdded,

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

.add-players {
  float: right;
}

</style>