# Gator UI Consistency Audit Report

## 🎨 **Font Size Standardization Needed**

### Current Usage Counts:
- `text-[28px]` - 1 use (Dashboard header) ❌
- `text-[24px]` - 2 uses (Tunnels/Routing headers) ❌  
- `text-[20px]` - 1 use (VPN card header) ❌
- `text-[18px]` - 1 use (System name) ❌
- `text-[14px]` - 10 uses ✅ (Body/primary)
- `text-[13px]` - 6 uses ✅ (Secondary)
- `text-[12px]` - 10 uses ✅ (Meta)
- `text-[11px]` - 5 uses ✅ (Small labels)
- `text-[10px]` - 14 uses ⚠️ (Too small)

### Recommended Standard:
- **Page Headers**: `text-[24px]` (currently mixed 24px/28px)
- **Card Headers**: `text-[18px]` (currently mixed 18px/20px)
- **Primary Text**: `text-[14px]` ✅
- **Secondary**: `text-[13px]` ✅
- **Meta/Small**: `text-[12px]` ✅
- **Labels**: `text-[11px]` ✅
- **Minimum**: `text-[11px]` (replace all 10px)

## 🎯 **Card Background Inconsistencies**

### Current Distribution:
- `bg-[var(--bg-tertiary)]` - 14 uses ✅ (Default cards)
- `bg-[var(--bg-hover)]` - 18 uses ✅ (Hover states)
- `bg-[var(--bg-secondary)]` - 12 uses ⚠️ (Should be tertiary)
- `bg-[var(--bg-elevated)]` - 1 use ⚠️ (Should be tertiary)

### Issues Found:
1. Some cards use `bg-secondary` instead of `tertiary`
2. Empty states use different backgrounds
3. Table containers inconsistent

## 📐 **Spacing Inconsistencies**

### Padding Variations:
- Cards: `p-3`, `p-4`, `p-5`, `p-6` (should standardize to `p-4`)
- Section gaps: `space-y-4`, `space-y-5`, `space-y-6` (standardize to `space-y-5`)
- Card gaps: `gap-3`, `gap-4` (standardize to `gap-4`)

## 🔧 **Other Issues**

1. **Border radius**: Mix of `rounded-lg` and `rounded-xl` on cards
2. **Toggle switches**: Different sizes (h-5 w-9 vs h-6 w-11)
3. **Status badges**: Some use custom styles instead of Badge component
4. **Icon sizes**: Mix of h-4, h-5 (standardize to h-4 for inline)

## ✅ **Recommended Fixes**

### Priority 1 (Critical):
1. Standardize all page headers to `text-[24px]`
2. Standardize card backgrounds to `bg-[var(--bg-tertiary)]`
3. Replace all `text-[10px]` with `text-[11px]`

### Priority 2 (Important):
1. Standardize card padding to `p-4`
2. Standardize section spacing to `space-y-5`
3. Fix toggle switch sizes

### Priority 3 (Nice to have):
1. Standardize border radius on cards
2. Audit all icon sizes
3. Fix remaining zinc color references

## 📁 **Files to Update**

1. Dashboard.tsx - Header size, font sizes
2. Routing.tsx - Header size, card backgrounds
3. Tunnels.tsx - Header size
4. VpnSetup.tsx - Header size
5. All table pages - Card backgrounds
6. AppCard.tsx - Already consistent ✅
