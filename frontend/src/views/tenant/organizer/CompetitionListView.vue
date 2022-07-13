<template>
  <div class="organizer-competition-list">
    <button class="add-competition" @click="handleAddCompetition">
      大会作成
    </button>
    <h2>
      大会一覧
    </h2>

    <TableBase
      :header="tableHeader"
      :data="tableData"
      :row-attr="competitions"
    >
      <template #cell-action="slotProps">
        <template v-if="!slotProps.row.is_finished">
          <button
            type="button"
            class="slim"
            :value="slotProps.row.id"
            @click="handleUploadCSV"
          >
            CSV入稿
          </button>
          <button
            type="button"
            class="slim"
            :value="slotProps.row.id"
            @click="handleCompleteCompetition"
          >
            完了にする
          </button>
        </template>
      </template>
    </TableBase>
  </div>
</template>

<script lang="ts">
import { ref, computed, onMounted, defineComponent } from 'vue'
import axios from 'axios'

import TableBase, { TableColumn } from '@/components/parts/TableBase.vue'


type Competition = {
  id: string
  title: string
  is_finished: boolean
}

export default defineComponent({
  name: 'CompetitionListView',
  components: {
    TableBase,
  },
  setup() {
    const competitions = ref<Competition[]>([])

    const fetchCompetitions = async () => {
      const res = await axios.get('/api/organizer/competitions')
      if (!res.data.status) {
        window.alert('failed to fetch competitions: status=false')
        return
      }

      console.log(res.data)
      competitions.value = res.data.data.competitions
    }

    onMounted(() => {
      fetchCompetitions()
    })

    const handleLoading = () => {
      return
    }

    const isLoading = ref(false)
    const noMoreLoad = ref(false)

    const handleAddCompetition = () => {
      console.log('add competition')
    }

    const handleUploadCSV = (evt: MouseEvent) => {
      const target = evt.target as HTMLButtonElement
      if (!target) return

      const competitionId = target.value

    }

    const handleCompleteCompetition = (evt: MouseEvent) => {
      const target = evt.target as HTMLButtonElement
      if (!target) return

      completeCompetition(target.value)
    }

    const completeCompetition = async (competitionId: string) => {
      const res = await axios.post(`/api/organizer/competition/${competitionId}/finish`)
      if (!res || !res.data.status) {
        window.alert('faied to finish')
        return
      }
      fetchCompetitions() // 画面を更新!
    }

    const tableHeader: TableColumn[] = [
      {
        width: '20%',
        align: 'center',
        text: '大会ID',
      },
      {
        width: '40%',
        align: 'left',
        text: '大会名',
      },
      {
        width: '20%',
        align: 'center',
        text: '完了ステータス',
      },
      {
        width: '20%',
        align: 'center',
        text: 'アクション',
      },
    ]

    const tableData = computed<string[][]>(() => 
      competitions.value.map(c => {
        return [
          c.id,
          c.title,
          c.is_finished ? '大会完了' : '開催中',
          '##slot##cell-action',
        ]
      })
    )

    return {
      competitions,
      isLoading,
      noMoreLoad,
      handleLoading,
      handleAddCompetition,
      handleUploadCSV,
      handleCompleteCompetition,

      tableHeader,
      tableData,
    }
  },
})
</script>

<style scoped>
.organizer-competition-list {
  padding: 0 20px 20px;
}

.add-competition {
  float: right;
}

</style>
