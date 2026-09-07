<template>
  <MDrawer
    :open="visible"
    icon="download"
    color="var(--violet)"
    :title="$t('ui.bulkNew')"
    :sub="$t('ui.bulkSub')"
    :save-label="$t('actions.save') + (outbounds.length > 0 ? ' (' + outbounds.length + ')' : '')"
    :width="500"
    :loading="loading"
    @close="closeModal"
    @save="saveChanges"
  >
    <!-- step 1: subscription link -->
    <template v-if="outbounds.length == 0">
      <Field :label="$t('client.sub')">
        <input class="input mono" dir="ltr" v-model="link" placeholder="http[s]://<domain>[:]<port>/<path>" />
      </Field>
      <div style="margin-bottom: 15px;">
        <CheckLabel v-model="addUrlTest" :label="$t('out.addUrlTest')" />
      </div>
      <div style="display: flex; justify-content: center; margin-bottom: 15px;">
        <Btn variant="primary" :loading="loading" @click="linkCheck">{{ $t('submit') }}</Btn>
      </div>
    </template>

    <!-- step 2: converted outbounds preview -->
    <template v-else>
      <div style="margin-bottom: 12px;">
        <Chip color="emerald" dot>{{ outbounds.length }} {{ $t('ui.linksFound') }}</Chip>
      </div>
      <div class="card" style="overflow: hidden; margin-bottom: 15px;">
        <div style="overflow-x: auto;">
          <table class="dtable">
            <thead>
              <tr>
                <th style="width: 36px;"></th>
                <th>{{ $t('type') }}</th>
                <th>{{ $t('objects.tag') }}</th>
                <th>{{ $t('out.addr') }}</th>
                <th>{{ $t('objects.tls') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(item, index) in outbounds" :key="index">
                <td>
                  <Ico v-if="outChecks[index] == 1" name="check" :size="15" style="color: var(--emerald);" />
                  <Ico v-else-if="outChecks[index] == 2" name="close" :size="15" style="color: var(--rose);" />
                  <Ico v-else-if="outChecks[index] == 3" name="refresh" :size="15" style="color: var(--text-3);" />
                  <Ico v-else name="dots" :size="15" style="color: var(--text-3);" />
                </td>
                <td>{{ item.type }}</td>
                <td class="mono" style="font-size: 12px;">{{ item.tag }}</td>
                <td class="mono" style="font-size: 12px;">{{ item.server }}{{ item.server_port ? ':' + item.server_port : '' }}</td>
                <td>{{ Object.hasOwn(item, 'tls') ? $t(item.tls?.enabled ? 'enable' : 'disable') : '-' }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </template>
  </MDrawer>
</template>

<script lang="ts" setup>
import { computed, ref, watch } from 'vue'
import HttpUtils from '@/plugins/httputil'
import RandomUtil from '@/plugins/randomUtil'
import Data from '@/store/modules/data'
import { Outbound, createOutbound } from '@/types/outbounds'
import MDrawer from '@/components/ui/MDrawer.vue'
import Field from '@/components/ui/Field.vue'
import Btn from '@/components/ui/Btn.vue'
import Ico from '@/components/ui/Ico.vue'
import Chip from '@/components/ui/Chip.vue'
import CheckLabel from '@/components/ui/CheckLabel.vue'

const props = defineProps<{
  visible: boolean
  outboundTags?: string[]
}>()
const emit = defineEmits<{ close: [] }>()

const loading = ref(false)
const link = ref('')
const outbounds = ref<Outbound[]>([])
const outChecks = ref<number[]>([])
const addUrlTest = ref(false)

const newOutboundTags = computed((): string[] => outbounds.value.map((o: Outbound) => o.tag))

const resetData = () => {
  outbounds.value = []
  outChecks.value = []
  link.value = ''
  addUrlTest.value = false
  loading.value = false
}

const closeModal = () => {
  resetData()
  emit('close')
}

const linkCheck = async () => {
  loading.value = true
  outbounds.value = []
  const msg = await HttpUtils.post('api/subConvert', { link: link.value })
  if (msg.success) {
    if (msg.obj?.length > 0) {
      msg.obj.forEach((o: any, index: number) => {
        if (newOutboundTags.value.includes(o.tag)) o.tag = o.tag + '-' + (index + 1)
        outbounds.value.push(createOutbound(o.type, o))
        outChecks.value.push(0)
      })
      if (addUrlTest.value) {
        const urlTestTsg = 'urltest-' + RandomUtil.randomSeq(3)
        outbounds.value.push(createOutbound('urltest', {
          tag: urlTestTsg,
          outbounds: outbounds.value.map((o: Outbound) => o.tag),
          interrupt_exist_connections: false,
          interval: '30s',
        }))
      }
    }
  }
  loading.value = false
}

const saveChanges = async () => {
  if (!props.visible || outbounds.value.length == 0) return
  // check duplicate tag
  outbounds.value.forEach((o: Outbound, index: number) => {
    const isDuplicatedTag = Data().checkTag('outbound', 0, o.tag)
    outChecks.value[index] = isDuplicatedTag ? 2 : 0
  })

  // save data
  //
  // Sequential and awaited. forEach(async ...) started every save at once and
  // then dropped loading before any of them answered, which re-enabled the save
  // button (Btn disables itself while loading) with the batch still in flight --
  // a second click re-ran the whole batch, and checkTag could not stop it,
  // because the store had not been refreshed with the rows just written.
  //
  // Each save also refreshes the whole panel and invalidates the server's config
  // cache, so a subscription with fifty outbounds meant fifty full config
  // rebuilds -- hence refresh=false here and one reload at the end.
  loading.value = true
  let saved = false
  for (let index = 0; index < outbounds.value.length; index++) {
    if (outChecks.value[index] == 2) continue
    outChecks.value[index] = 3
    const success = await Data().save('outbounds', 'new', outbounds.value[index], undefined, false)
    outChecks.value[index] = success ? 1 : 2
    if (success) saved = true
  }
  if (saved) await Data().loadData(true)
  loading.value = false
}

watch(() => props.visible, (v) => {
  if (v) resetData()
})
</script>
