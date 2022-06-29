<template>
  <div class="organizer">
    Organizer向けのメニュー画面だよ
    <ul>
      <li>
        <router-link to="/organizer/players">プレイヤー一覧</router-link>
      </li>
      <li>
        <router-link to="/organizer/competitions">大会一覧</router-link>
      </li>
      <li>
        <router-link to="/organizer/billing">請求情報</router-link>
      </li>
    </ul>
  </div>
</template>

<script lang="ts">
import { defineComponent } from 'vue'

export default defineComponent({
  setup() {
    return {}
  },
})
</script>
