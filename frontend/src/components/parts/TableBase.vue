<template>
  <table>
    <thead>
      <tr>
        <th
          v-for="(h, index) in header"
          :key="index"
          :style="{ textAlign: 'center', width: h.width }"
        >
          {{ h.text }}
        </th>
      </tr>
    </thead>
    <tbody>
      <tr
        v-for="(d, i) in data"
        :key="`idx-${i}`"
      >
        <td
          v-for="(c, index) in d"
          :key="`idx-${i}-${index}`"
          :style="{ textAlign: header[index].align, width: header[index].width }"
        >
          <template v-if="c.startsWith('##slot##')">
            <slot
              :name="c.replace('##slot##', '')"
              :row="rowAttr[i]"
            >
            </slot>
          </template>
          <template v-else>
            {{ c }}
          </template>
        </td>
      </tr>
    </tbody>
    <tfoot>
      <slot name="footer" />
    </tfoot>
  </table>

</template>

<script lang="ts">
import { defineComponent } from 'vue'

export type TableColumn = {
  width: string
  align: 'left' | 'center' | 'right'
  text: string
}

type Props = {
  header: TableColumn[]
  data: string[][]
  rowAttr: any[]
}

export default defineComponent({
  name: 'TableBase',
  props: {
    header: {
      type: Array as () => TableColumn[],
      default: () => [],
    },
    data: {
      type: Array as () => string[][],
      default: () => [],
    },
    rowAttr: {
      type: Array as () => any[],
      default: () => [],
    },
  },
  setup(props: Props) {
    return {
    }
  },
})
</script>


<style scoped>
table {
  border: 1px solid lightgray;
  border-collapse: collapse;
  width: 100%;
}

th, td {
  padding: 4px 8px;
  border: 1px solid lightgray;
  background: rgba(48,48,64,0.3);
}
th {
  background: rgba(32,32,48,0.8);
}

a:link, a:visited {
  text-decoration: underline;
  color: white;
}
</style>