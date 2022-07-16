import { ref, onMounted } from 'vue'
import axios from 'axios'
import { routeLocationKey } from 'vue-router'

type MeResponse = {
  data: {
    tenant: {
      name: string
      display_name: string
    }
    logged_in: boolean
    role: 'admin' | 'organizer' | 'player' | 'none'
  }
  status: boolean
}

export const useLoginStatus = () => {
  const isLoggedIn = ref<boolean|undefined>()
  const tenantDisplayName = ref('')
  const role = ref<string>('none')

  const fetch = async () => {
    const res = await axios.get('/api/me')
    const payload: MeResponse = res.data

    isLoggedIn.value = payload.data.logged_in
    role.value = payload.data.role
    tenantDisplayName.value = payload.data.tenant.display_name
  }

  onMounted(() => {
    fetch()
  })

  const refetch = async () => {
    await fetch()
  }

  return {
    tenantDisplayName,
    isLoggedIn,
    role,
    refetch,
  }
}
