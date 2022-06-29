<template>
  <div class="tenant-billing">
    <h2>
      請求情報
    </h2>

    <p>請求額: <span class="total-billing">{{ totalBilling.toLocaleString() }}円</span> になりま～す</p>

    <h3>大会ごとの請求明細</h3>
    <p>大会閲覧者数とは、スコア登録をしたプレイヤー以外で大会情報を閲覧した人です。大会ごとの請求額は、スコアを登録した人1人あたり100円、大会閲覧のみした人1人あたり10円で、合計金額が請求額となります。</p>

    <table class="billing-list">
      <thead>
        <tr>
          <th class="competition-id">大会ID</th>
          <th class="competition-title">大会名</th>
          <th class="player-count">スコア登録人数</th>
          <th class="visitor-count">大会閲覧者数</th>
          <th class="billing-yen">請求額</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="b in billingReports"
          :key="b.competition_id"
        >
          <td class="competition-id">{{ b.competition_id }}</td>
          <td class="competition-title">{{ b.competition_title }}</td>
          <td class="player-count">{{ b.player_count }}人 <span class="supplement">({{ b.billing_player_yen.toLocaleString() }}円)</span></td>
          <td class="visitor-count">{{ b.visitor_count }}人 <span class="supplement">({{ b.billing_visitor_yen.toLocaleString() }}円)</span></td>
          <td class="billing-yen">{{ b.billing_yen.toLocaleString() }}円</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script lang="ts">
import { ref, computed, onMounted, defineComponent } from 'vue'
import axios from 'axios'

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
  setup() {
    const billingReports = ref<BillingReport[]>([])
    const fetchBilling = async () => {
      const res = await axios.get('/api/organizer/billing')

      console.log(res)
      billingReports.value = res.data.data.reports
    }

    onMounted(() => {
      fetchBilling()
    })

    const totalBilling = computed(() =>
      billingReports.value.reduce((prev, cur) => prev + cur.billing_yen, 0)
    )

    return {
      totalBilling,
      billingReports,
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

.billing-list {
  border: 1px solid lightgray;
  border-collapse: collapse;
  width: 100%;
}

th, td {
  padding: 4px;
  border: 1px solid gray;
}

.competition-id {
  width: 15%;
  text-align: center;
}

.player-count,
.visitor-count,
.billing-yen {
  width: 15%;
  text-align: right;
}

.competition-title {
  width: 40%;
}

th.player-count,
th.visitor-count,
th.billing-yen {
  text-align: center;
}


.supplement {
  font-size: 0.75em;
}
</style>