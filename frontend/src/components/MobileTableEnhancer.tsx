import { useEffect } from 'react'

const READY_CLASS = 'peopleops-mobile-card-table-ready'
const HOST_CLASS = 'peopleops-mobile-table-card-host'
const LABEL_ATTRIBUTE = 'data-label'
const ACTIONS_ATTRIBUTE = 'data-mobile-actions'
const PRIMARY_ATTRIBUTE = 'data-mobile-primary'

function normalizeLabel(value: string) {
  return value.replace(/\s+/g, ' ').trim()
}

function getHeaderLabels(wrapper: HTMLElement) {
  const headerRows = Array.from(wrapper.querySelectorAll<HTMLTableRowElement>('.ant-table-thead > tr'))
  const headerRow = headerRows[headerRows.length - 1]

  if (!headerRow) return []

  return Array.from(headerRow.querySelectorAll<HTMLTableCellElement>(':scope > th')).map(header =>
    normalizeLabel(header.innerText || header.textContent || ''),
  )
}

function isDataRow(row: HTMLTableRowElement) {
  return (
    !row.classList.contains('ant-table-placeholder') &&
    !row.classList.contains('ant-table-measure-row') &&
    !row.classList.contains('ant-table-expanded-row') &&
    row.querySelector(':scope > td') !== null
  )
}

function isActionCell(cell: HTMLTableCellElement, label: string) {
  return (
    label.includes('操作') ||
    Boolean(cell.querySelector('.ant-btn, button, .ant-dropdown-trigger, .ant-popconfirm'))
  )
}

function isSelectableCell(cell: HTMLTableCellElement) {
  return Boolean(cell.querySelector('.ant-checkbox-wrapper, input[type="checkbox"], input[type="radio"]'))
}

function applyMobileTableLabels() {
  document.querySelectorAll<HTMLElement>(`.${HOST_CLASS}`).forEach(host => {
    host.classList.remove(HOST_CLASS)
  })

  document.querySelectorAll<HTMLElement>('.ant-table-wrapper').forEach(wrapper => {
    const labels = getHeaderLabels(wrapper)
    const rows = Array.from(wrapper.querySelectorAll<HTMLTableRowElement>('.ant-table-tbody > tr')).filter(isDataRow)

    rows.forEach(row => {
      const cells = Array.from(row.querySelectorAll<HTMLTableCellElement>(':scope > td'))
      const primaryIndex = cells.findIndex((cell, index) => {
        const label = labels[index] || ''
        return label && !isActionCell(cell, label) && !isSelectableCell(cell)
      })

      cells.forEach((cell, index) => {
        const label = labels[index] || ''
        cell.setAttribute(LABEL_ATTRIBUTE, label)

        if (isActionCell(cell, label)) {
          cell.setAttribute(ACTIONS_ATTRIBUTE, 'true')
        } else {
          cell.removeAttribute(ACTIONS_ATTRIBUTE)
        }

        if (index === primaryIndex) {
          cell.setAttribute(PRIMARY_ATTRIBUTE, 'true')
        } else {
          cell.removeAttribute(PRIMARY_ATTRIBUTE)
        }
      })
    })

    const ready = labels.length > 0 && rows.length > 0
    wrapper.classList.toggle(READY_CLASS, ready)

    if (ready) {
      wrapper.closest<HTMLElement>('.ant-card')?.classList.add(HOST_CLASS)
    }
  })
}

export default function MobileTableEnhancer() {
  useEffect(() => {
    let frame = 0
    const schedule = () => {
      if (frame) return

      frame = window.requestAnimationFrame(() => {
        frame = 0
        applyMobileTableLabels()
      })
    }

    schedule()

    const observer = new MutationObserver(schedule)
    observer.observe(document.body, {
      childList: true,
      subtree: true,
      characterData: true,
    })

    window.addEventListener('resize', schedule)
    window.addEventListener('orientationchange', schedule)

    return () => {
      if (frame) window.cancelAnimationFrame(frame)
      observer.disconnect()
      window.removeEventListener('resize', schedule)
      window.removeEventListener('orientationchange', schedule)
    }
  }, [])

  return null
}
