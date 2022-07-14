<template>
  <div class="admin">
    <button class="add-tenant" @click="handleAddTenant">
      テナント追加
    </button>
    <AddTenantModal
      v-show="showModal"
      @close="handleAddTenantClose"
      @tenantAdded="handleTenantAdded"
    />

    <h2>テナント一覧</h2>
    <TableBase
      :header="tableHeader"
      :data="tableData"
      :row-attr="tenants"
    >
      <template #cell-name="slotProps">
        <a :href="`https://${slotProps.row.name}.t.isucon.dev/`">{{ slotProps.row.name }}</a>
      </template>
      <template #footer>
        <tr v-if="!noMoreLoad">
          <td colspan="4" class="loading">
            <template v-if="isLoading">
              読み込み中... <span class="cycle-loop">の</span>
            </template>
            <template v-else>
            <button
              type="button"
              class="slim"
              @click="handleLoading"
            >さらに読み込む</button>
            </template>
          </td>
        </tr>
      </template>
    </TableBase>
  </div>
</template>

<script lang="ts">
import { ref, computed, onMounted, defineComponent } from 'vue'
import axios from 'axios'

import TableBase, { TableColumn } from '@/components/parts/TableBase.vue'
import AddTenantModal from '@/components/admin/AddTenantModal.vue'

import { useLoginStatus } from '@/assets/hooks/useLoginStatus'


type Tenant = {
  id: string
  name: string
  display_name: string
  billing: number
}

type BillingResponse = {
  data: { tenants: Tenant[] }
  status: boolean
}

export default defineComponent({
  components: {
    TableBase,
    AddTenantModal,
  },
  props: {
    isLoggedIn: {
      type: Boolean,
      required: true,
    },
  },
  setup() {
    const { isLoggedIn } = useLoginStatus()

    const tenants = ref<Tenant[]>([])
    const beforeCursor = ref('')
    const isLoading = ref(false)
    const noMoreLoad = ref(false)

    const fetch = async (before: string) => {
      const params = new URLSearchParams()
      if (before) {
        params.append('before', before)
      }

      isLoading.value = true
      try {
        const billing = await axios.get('/api/admin/tenants/billing', {
          params,
        })

        const res: BillingResponse = billing.data
        if (!res.status) {
          window.alert('Failed to fetch tenant list: status=false')
          return
        }

        if (!res.data.tenants.length) {
          noMoreLoad.value = true
          return
        }

        tenants.value.push(...res.data.tenants)
        beforeCursor.value = res.data.tenants[res.data.tenants.length-1].id

        if (res.data.tenants.length < 10) {
          noMoreLoad.value = true
        }
      } catch (e: any) {
        window.alert('Failed to fetch tenants list')
      } finally {
        isLoading.value = false
      }
    }

    onMounted(() => {
      setTimeout(() => {
        if (isLoggedIn.value) {
          fetch('')
        }
      }, 250)
    })

    const handleLoading = () => {
      fetch(beforeCursor.value)
    }

    const showModal = ref(false)
    const handleAddTenant = () => {
      showModal.value = true
    }
    const handleAddTenantClose = () => {
      showModal.value = false
    }

    const handleTenantAdded = (tenant: Tenant) => {
      tenants.value.unshift(tenant)
    }

    const tableHeader: TableColumn[] = [
      {
        width: '10%',
        align: 'center',
        text: 'ID',
      },
      {
        width: '20%',
        align: 'center',
        text: 'テナント名',
      },
      {
        width: '45%',
        align: 'left',
        text: '表示名',
      },
      {
        width: '15%',
        align: 'right',
        text: '請求額',
      },
    ]

    const tableData = computed<string[][]>(() => 
      tenants.value.map(t => {
        return [
          t.id,
          '##slot##cell-name',
          t.display_name,
          t.billing.toLocaleString() + '円'
        ]
      })
    )

    return {
      tenants,
      handleLoading,
      isLoading,
      noMoreLoad,
      showModal,
      handleAddTenant,
      handleAddTenantClose,
      handleTenantAdded,
      tableHeader,
      tableData,
    }
  },
})
</script>

<style scoped>
.admin {
  padding: 0 20px 20px;
  margin: 0 auto;
}

.add-tenant {
  float: right;
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
