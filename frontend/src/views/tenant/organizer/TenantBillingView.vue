<template>
  <div class="tenant-billing">
    <h2>
      請求情報
    </h2>

    <template v-if="isLoading">
      <p>読み込み中… (とても長くなることがあります)</p>
    </template>
    <template v-else>
      <p>請求額: <span class="total-billing">{{ totalBilling.toLocaleString() }}円</span> になりま～す</p>

      <h3>大会ごとの請求明細</h3>
      <p>大会閲覧者数とは、スコア登録をしたプレイヤー以外で大会情報を閲覧した人です。大会ごとの請求額は、スコアを登録した人1人あたり100円、大会閲覧のみした人1人あたり10円で、合計金額が請求額となります。</p>
      <p>大会が未完了の場合は0人(0円)になります。大会が完了しているかどうかのステータスは <router-link to="/organizer/competitions">大会一覧</router-link> 画面を参照してください。</p>

      <TableBase
        :header="tableHeader"
        :data="tableData"
        :row-attr="billingReports"
      >
        <template #cell-player-count="slotProps">
          {{ slotProps.row.player_count }}人 <span class="supplement">({{ slotProps.row.billing_player_yen.toLocaleString() }}円)</span>
        </template>
        <template #cell-visitor-count="slotProps">
          {{ slotProps.row.visitor_count }}人 <span class="supplement">({{ slotProps.row.billing_visitor_yen.toLocaleString() }}円)</span>
        </template>
      </TableBase>
    </template>
  </div>
</template>

<script lang="ts">
import { ref, computed, onMounted, defineComponent } from 'vue'
import axios from 'axios'

import TableBase, { TableColumn } from '@/components/parts/TableBase.vue'

type BillingReport = {
  competition_id: string
  competition_title: string
  player_count: number
  visitor_count: number
  billing_player_yen: number
  billing_visitor_yen: number
  billing_yen: number
}

export default defineComponent({
  components: {
    TableBase,
  },
  setup() {
    const isLoading = ref(true)
    const billingReports = ref<BillingReport[]>([])
    const fetchBilling = async () => {
      const res = await axios.get('/api/organizer/billing')

      billingReports.value = res.data.data.reports
      isLoading.value = false
    }

    onMounted(() => {
      fetchBilling()
    })

    const totalBilling = computed(() =>
      billingReports.value.reduce((prev, cur) => prev + cur.billing_yen, 0)
    )

    const tableHeader: TableColumn[] = [
      {
        width: '15%',
        align: 'center',
        text: '大会ID',
      },
      {
        width: '35%',
        align: 'left',
        text: '大会名',
      },
      {
        width: '18%',
        align: 'right',
        text: 'スコア登録人数',
      },
      {
        width: '18%',
        align: 'right',
        text: '大会閲覧者数',
      },
      {
        width: '14%',
        align: 'right',
        text: '請求額',
      },
    ]

    const tableData = computed<string[][]>(() => 
      billingReports.value.map(br => {
        return [
          br.competition_id,
          br.competition_title,
          '##slot##cell-player-count',
          '##slot##cell-visitor-count',
          br.billing_yen.toLocaleString() + '円'
        ]
      })
    )


    return {
      isLoading,
      totalBilling,
      billingReports,
      tableHeader,
      tableData,
    }
  },
})
</script>

<style scoped>
.total-billing {
  font-size: 2.5em;
  font-weight: bold;
}

.tenant-billing {
  padding: 0 20px 20px;
}

.supplement {
  font-size: 0.75em;
}
</style>