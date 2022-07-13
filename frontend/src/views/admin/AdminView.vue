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
    <table class="tenant-list">
      <thead>
        <tr>
          <th class="tenant-id">ID</th>
          <th class="tenant-name">テナント名</th>
          <th class="tenant-display-name">表示名</th>
          <th class="billing">請求額</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="t in tenants"
          :key="t.id"
        >
          <td class="tenant-id">{{ t.id }}</td>
          <td class="tenant-name">{{ t.name }} <a :href="`https://${t.name}.t.isucon.dev/`">→</a></td>
          <td class="tenant-display-name">{{ t.display_name }}</td>
          <td class="billing">{{ t.billing.toLocaleString() }}円</td>
        </tr>
        <tr v-if="!noMoreLoad">
          <td colspan="4" class="loading">
            <template v-if="isLoading">
              読み込み中... <span class="cycle-loop">の</span>
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
import axios from 'axios'

import AddTenantModal from '@/components/admin/AddTenantModal.vue'

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
    AddTenantModal,
  },
  setup() {
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

        if (res.data.tenants.length < 20) {
          noMoreLoad.value = true
        }
      } catch (e: any) {
        window.alert('Failed to fetch tenants list')
      } finally {
        isLoading.value = false
      }
    }

    onMounted(() => {
      fetch('')
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

    return {
      tenants,
      handleLoading,
      isLoading,
      noMoreLoad,
      showModal,
      handleAddTenant,
      handleAddTenantClose,
      handleTenantAdded,
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

.tenant-list {
  border: 1px solid gray;
  border-collapse: collapse;
  width: 100%;
}

th, td {
  padding: 4px;
  border: 1px solid lightgray;
}

.tenant-id {
  width: 10%;
  text-align: center;
}

.tenant-name {
  width: 30%;
}

.tenant-display-name {
  width: 45%
}

.billing {
  width: 15%;
}

td.billing {
  text-align: right;
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
