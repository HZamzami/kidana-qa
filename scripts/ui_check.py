"""Full UI walkthrough (run with `just ui-check`).

Drives the running app like a user: every page at phone (390px) and desktop
(1366px) widths, all three roles, happy paths and error paths (validation,
403/404, login failure), photo upload, Excel template/import/export, print
report, findings lifecycle and admin settings. Screenshots go to
tmp/ui-check/<NN>-<name>.png; at the end it prints the failed steps and every
browser console error / failed request.

Uses the dev database and leaves DEMO-*/IMP-* records behind on purpose.
"""
import io
import os
import struct
import sys
import time
import traceback
import zlib

from playwright.sync_api import sync_playwright

BASE = os.environ.get("BASE_URL", "http://localhost:8080")
EMAIL = os.environ.get("ADMIN_EMAIL", "admin@kidana.local")
PASSWORD = os.environ.get("ADMIN_PASSWORD", "change-me-now")
OUT = sys.argv[1]
TAG = str(int(time.time()) % 100000)
REF = f"DEMO-{TAG}"

problems, failures = [], []
n = [0]


def shot(page, name, full=True):
    n[0] += 1
    page.screenshot(path=f"{OUT}/{n[0]:02d}-{name}.png", full_page=full)


def watch(page, tag):
    # 4xx "Failed to load resource" lines come from the deliberate error-path tests.
    page.on("console", lambda m: m.type in ("error", "warning") and "Failed to load resource" not in m.text
            and problems.append(f"[{tag}] console {m.type}: {m.text}"))
    page.on("pageerror", lambda e: problems.append(f"[{tag}] pageerror: {e}"))
    page.on("response", lambda r: r.status >= 500 and problems.append(f"[{tag}] HTTP {r.status} {r.url}"))


def step(name):
    def deco(fn):
        def run(*a, **kw):
            try:
                return fn(*a, **kw)
            except Exception as e:  # keep going; report at the end
                failures.append(f"{name}: {e.__class__.__name__}: {str(e).splitlines()[0]}")
                traceback.print_exc(limit=1)
        return run
    return deco


def png_bytes(w=64, h=48, rgb=(181, 150, 102)):
    raw = b"".join(b"\x00" + bytes(rgb) * w for _ in range(h))
    def chunk(t, d):
        return struct.pack(">I", len(d)) + t + d + struct.pack(">I", zlib.crc32(t + d) & 0xFFFFFFFF)
    return b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", w, h, 8, 2, 0, 0, 0)) + chunk(b"IDAT", zlib.compress(raw)) + chunk(b"IEND", b"")


def login(ctx, email, password, tag):
    page = ctx.new_page()
    watch(page, tag)
    page.goto(BASE + "/login")
    page.fill("#email", email)
    page.fill("#password", password)
    page.click("main button[type=submit]")
    page.wait_for_load_state("networkidle")
    return page


def visible_section(p):
    return p.locator("section[x-data=criterionCard]:visible")


def rate_flow(pp, ratings, finding_on=None):
    """ratings: list of labels per criterion; finding_on: index to add a finding on."""
    for i, label in enumerate(ratings):
        sec = visible_section(pp)
        sec.locator(f"label.rate:has(span:text-is('{label}'))").click()
        pp.wait_for_timeout(250)
        if finding_on == i:
            sec.locator("text=＋ تسجيل ملاحظة / عدم مطابقة").click()
            sec.locator("textarea[name=description]").fill("الفني فتح اللوحة دون تطبيق العزل والإقفال (LOTO)")
            sec.locator("textarea[name=corrective_action]").fill("إيقاف العمل فوراً وتطبيق LOTO وتوعية الفريق")
            sec.locator("input[name=owner]").fill("مشرف المقاول")
            sec.locator("input[name=due_date]").fill("2026-11-30")
            sec.locator("text=حفظ الملاحظة").click()
            pp.wait_for_timeout(600)
        btn = "المراجعة" if i == len(ratings) - 1 else "التالي"
        visible_section(pp).locator(f"button:text-is('{btn}')").click()
        pp.wait_for_timeout(300)
    pp.wait_for_timeout(1000)


with sync_playwright() as p:
    browser = p.chromium.launch()
    desk = browser.new_context(viewport={"width": 1366, "height": 900}, locale="ar-SA", accept_downloads=True)
    phone = browser.new_context(viewport={"width": 390, "height": 844}, device_scale_factor=2, locale="ar-SA", is_mobile=True, has_touch=True)
    state = {}

    @step("login page + failed login")
    def s_login():
        pg = desk.new_page(); watch(pg, "anon")
        pg.goto(BASE + "/")  # redirect to login
        shot(pg, "login", full=False)
        pg.fill("#email", EMAIL); pg.fill("#password", "wrong-password")
        pg.click("main button[type=submit]"); pg.wait_for_load_state("networkidle")
        shot(pg, "login-wrong-password", full=False)
        pp = phone.new_page(); watch(pp, "anon-phone")
        pp.goto(BASE + "/login"); shot(pp, "phone-login", full=False); pp.close()
        pg.close()
    s_login()

    page = login(desk, EMAIL, PASSWORD, "admin")
    state["admin_cookies"] = desk.cookies()
    phone.add_cookies(state["admin_cookies"])
    pp = phone.new_page(); watch(pp, "admin-phone")

    @step("empty-ish dashboard + work orders list")
    def s_lists():
        page.goto(BASE + "/"); shot(page, "dashboard-start")
        page.goto(BASE + "/work-orders"); shot(page, "work-orders-list")
        pp.goto(BASE + "/work-orders"); shot(pp, "phone-work-orders")
    s_lists()

    @step("work order form validation")
    def s_wo_invalid():
        page.goto(BASE + "/work-orders/new")
        page.click("main button[type=submit]"); page.wait_for_load_state("networkidle")
        shot(page, "work-order-form-errors")
    s_wo_invalid()

    @step("create work order")
    def s_wo_create():
        page.goto(BASE + "/work-orders/new")
        page.select_option("#f_type", "WO")
        page.fill("#f_ref", REF)
        page.select_option("#f_priority", "high")
        page.select_option("#f_site_id", index=1)
        page.fill("#f_location", "مخيم 312 — الوحدة الخارجية")
        page.select_option("#f_system_id", index=1)
        page.fill("#f_asset", "AC-MN-0312")
        page.fill("#f_technician", "محمد علي")
        page.fill("#f_planned_date", "2026-11-20")
        page.fill("#f_actual_date", "2026-11-21")
        page.fill("#f_description", "صيانة تصحيحية لوحدة تكييف لا تبرد")
        shot(page, "work-order-form-filled")
        page.click("main button[type=submit]"); page.wait_for_load_state("networkidle")
        state["wo_url"] = page.url
        shot(page, "work-order-created")
        pp.goto(page.url); shot(pp, "phone-work-order")
    s_wo_create()

    @step("duplicate ref rejected")
    def s_wo_dup():
        page.goto(BASE + "/work-orders/new")
        page.fill("#f_ref", REF.lower())
        page.select_option("#f_site_id", index=1); page.select_option("#f_system_id", index=1)
        page.click("main button[type=submit]"); page.wait_for_load_state("networkidle")
        shot(page, "work-order-duplicate-ref", full=False)
    s_wo_dup()

    @step("edit work order")
    def s_wo_edit():
        page.goto(state["wo_url"] + "/edit")
        shot(page, "work-order-edit")
        page.fill("#f_technician", "محمد علي — فني تكييف")
        page.click("main button[type=submit]"); page.wait_for_load_state("networkidle")
        shot(page, "work-order-after-edit", full=False)
    s_wo_edit()

    @step("QA inspection on phone (critical fail, finding, photo)")
    def s_qa():
        page.goto(state["wo_url"])
        page.click("text=بدء رقابة أثناء التنفيذ"); page.wait_for_load_state("networkidle")
        state["qa_url"] = page.url
        pp.goto(state["qa_url"]); pp.wait_for_load_state("networkidle")
        shot(pp, "phone-qa-start", full=False)
        # photo on the first criterion
        sec = visible_section(pp)
        sec.locator("input[type=file]").set_input_files({"name": "loto.png", "mimeType": "image/png", "buffer": png_bytes()})
        pp.wait_for_timeout(1200)
        shot(pp, "phone-qa-photo-uploaded")
        # wrong file type
        sec.locator("input[type=file]").set_input_files({"name": "note.txt", "mimeType": "text/plain", "buffer": b"not an image"})
        pp.wait_for_timeout(1000)
        shot(pp, "phone-qa-bad-file-toast", full=False)
        rate_flow(pp, ["غير مطابق", "مطابق", "مطابق جزئياً", "مطابق", "مطابق"], finding_on=0)
        shot(pp, "phone-qa-review")
        pp.click("text=إرسال الفحص"); pp.wait_for_load_state("networkidle")
        shot(pp, "phone-after-qa-submit")
    s_qa()

    @step("submit refused without finding (QC fail)")
    def s_qc_fail():
        page.goto(state["wo_url"])
        page.click("text=بدء فحص بعد التنفيذ"); page.wait_for_load_state("networkidle")
        state["qc1_url"] = page.url
        pp.goto(state["qc1_url"]); pp.wait_for_load_state("networkidle")
        count = pp.locator("nav[aria-label='البنود'] button").count() - 1
        rate_flow(pp, ["غير مطابق"] + ["مطابق"] * (count - 1))
        pp.click("text=إرسال الفحص"); pp.wait_for_load_state("networkidle")
        shot(pp, "phone-qc-submit-refused")
        # add a general finding from the review step, then submit
        box = pp.locator("#finding-box-general")
        box.locator("text=＋ تسجيل ملاحظة / عدم مطابقة").click()
        box.locator("textarea[name=description]").fill("الوحدة لا تصل لدرجة الحرارة المطلوبة")
        box.locator("textarea[name=corrective_action]").fill("إعادة شحن الفريون وفحص الضاغط")
        box.locator("text=حفظ الملاحظة").click(); pp.wait_for_timeout(800)
        pp.click("text=إرسال الفحص"); pp.wait_for_load_state("networkidle")
    s_qc_fail()

    @step("QC re-inspection passes (attempt 2)")
    def s_qc_pass():
        page.goto(state["wo_url"])
        page.click("text=بدء فحص بعد التنفيذ"); page.wait_for_load_state("networkidle")
        pp.goto(page.url); pp.wait_for_load_state("networkidle")
        count = pp.locator("nav[aria-label='البنود'] button").count() - 1
        rate_flow(pp, ["مطابق"] * (count - 1) + ["لا ينطبق"])
        shot(pp, "phone-qc2-review")
        pp.click("text=إرسال الفحص"); pp.wait_for_load_state("networkidle")
        page.goto(state["wo_url"]); shot(page, "work-order-timeline")
    s_qc_pass()

    @step("inspection read-only summary")
    def s_summary():
        page.goto(state["qa_url"]); shot(page, "qa-summary-desktop")
        pp.goto(state["qa_url"]); shot(pp, "phone-qa-summary")
    s_summary()

    @step("findings: list, edit validation, close, reopen, close")
    def s_findings():
        page.goto(BASE + "/findings"); shot(page, "findings-open")
        pp.goto(BASE + "/findings"); shot(pp, "phone-findings")
        page.locator("a:has-text('تحديث')").first.click(); page.wait_for_load_state("networkidle")
        shot(page, "finding-edit")
        page.fill("#f_description", ""); page.locator("form[action$='/close'] ~ *").count()
        page.locator("main form button:has-text('حفظ التعديلات')").click(); page.wait_for_load_state("networkidle")
        shot(page, "finding-edit-errors", full=False)
        page.fill("#f_description", "الفني فتح اللوحة دون تطبيق العزل والإقفال (LOTO)")
        page.locator("main form button:has-text('حفظ التعديلات')").click(); page.wait_for_load_state("networkidle")
        page.locator("button:has-text('إغلاق الملاحظة')").click(); page.wait_for_load_state("networkidle")
        shot(page, "finding-close-needs-note", full=False)
        page.fill("#f_closure_note", "تم تطبيق LOTO وتوقيع محضر توعية")
        page.locator("button:has-text('إغلاق الملاحظة')").click(); page.wait_for_load_state("networkidle")
        shot(page, "work-order-after-close-1")
        # close the second open finding → work order should become معتمد
        page.goto(BASE + "/findings")
        while page.locator("a:has-text('تحديث')").count():
            page.locator("a:has-text('تحديث')").first.click(); page.wait_for_load_state("networkidle")
            page.fill("#f_closure_note", "تمت المعالجة والتحقق")
            page.locator("button:has-text('إغلاق الملاحظة')").click(); page.wait_for_load_state("networkidle")
            page.goto(BASE + "/findings")
        page.goto(state["wo_url"]); shot(page, "work-order-accepted")
        page.goto(BASE + "/findings?scope=closed"); shot(page, "findings-closed")
        page.locator("a:has-text('عرض')").first.click(); page.wait_for_load_state("networkidle")
        shot(page, "finding-closed-view")
    s_findings()

    @step("work order list filters (htmx)")
    def s_filters():
        page.goto(BASE + "/work-orders")
        page.fill("#flt_q", REF); page.wait_for_timeout(900)
        shot(page, "work-orders-search")
        page.select_option("#flt_status", "accepted"); page.wait_for_timeout(900)
        shot(page, "work-orders-filter-accepted", full=False)
    s_filters()

    @step("dashboard with data + filters")
    def s_dash():
        page.goto(BASE + "/"); shot(page, "dashboard")
        page.select_option("#flt_stage", "qc"); page.wait_for_timeout(1000)
        shot(page, "dashboard-qc-only")
        pp.goto(BASE + "/"); shot(pp, "phone-dashboard")
    s_dash()

    @step("print report")
    def s_print():
        # leave one open finding (on a QA draft) so the report's findings table has a row
        page.goto(state["wo_url"])
        page.click("text=بدء رقابة أثناء التنفيذ"); page.wait_for_load_state("networkidle")
        page.click("nav[aria-label='البنود'] >> text=مراجعة"); page.wait_for_timeout(1200)
        box = page.locator("#finding-box-general")
        box.locator("text=＋ تسجيل ملاحظة / عدم مطابقة").click()
        box.locator("textarea[name=description]").fill("سلم الوصول للوحدة الخارجية غير مثبت")
        box.locator("textarea[name=corrective_action]").fill("تثبيت السلم وتوفير حزام أمان")
        box.locator("input[name=owner]").fill("مشرف السلامة")
        box.locator("input[name=due_date]").fill("2026-11-01")
        box.locator("input[name=critical]").check()
        box.locator("text=حفظ الملاحظة").click(); page.wait_for_timeout(800)
        page.goto(BASE + "/reports/print"); shot(page, "print-report")
        page.emulate_media(media="print"); shot(page, "print-report-print-media"); page.emulate_media(media="screen")
    s_print()

    @step("data page: template download, import, export")
    def s_data():
        page.goto(BASE + "/data"); shot(page, "data-page")
        with page.expect_download() as d:
            page.click("text=تنزيل قالب أوامر العمل")
        tpl = f"/tmp/template-{TAG}.xlsx"
        d.value.save_as(tpl)
        import openpyxl  # installed by the just recipe
        wb = openpyxl.load_workbook(tpl)
        ws = wb["أوامر العمل"]
        ws.append([f"IMP-{TAG}-1", "صيانة وقائية", "عرفات", "", "الكهرباء", "لوحة 7", f"مؤسسة تجريبية {TAG}", "", "متوسط", "05/11/2026", "06/11/2026", "صيانة دورية"])
        ws.append([f"IMP-{TAG}-2", "أمر عمل", "موقع جديد {TAG}", "", "المصاعد", "", "", "", "طارئ", "31/02/2026", "", ""])
        ws.append(["", "أمر عمل", "منى"])
        buf = io.BytesIO(); wb.save(buf)
        page.set_input_files("#import_file", {"name": "work-orders.xlsx", "mimeType": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "buffer": buf.getvalue()})
        page.click("main form button:has-text('استيراد')"); page.wait_for_load_state("networkidle")
        page.locator("details summary").first.click() if page.locator("details:not([open]) summary").count() else None
        shot(page, "import-work-orders-result")

        # unrecognised file
        bad = openpyxl.Workbook(); bad.active.append(["foo", "bar"]); b2 = io.BytesIO(); bad.save(b2)
        page.set_input_files("#import_file", {"name": "random.xlsx", "mimeType": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "buffer": b2.getvalue()})
        page.click("main form button:has-text('استيراد')"); page.wait_for_load_state("networkidle")
        shot(page, "import-unrecognised", full=False)

        with page.expect_download() as d:
            page.click("text=تصدير كل البيانات (Excel)")
        xp = f"/tmp/export-{TAG}.xlsx"
        d.value.save_as(xp)
        xw = openpyxl.load_workbook(xp)
        sheets = xw.sheetnames
        ins = xw["الفحوصات"]
        hdr = [c.value for c in ins[1]]
        print("EXPORT sheets:", sheets, "| inspections rows:", ins.max_row - 1, "| first headers:", hdr[:14])
        print("EXPORT first data row:", [c.value for c in ins[2]][:16])
    s_data()

    @step("admin: lookups, users, checklists")
    def s_admin():
        page.goto(BASE + "/admin"); shot(page, "admin-home")
        page.goto(BASE + "/admin/lookups/sites"); shot(page, "admin-sites")
        page.click("main form button:has-text('إضافة')"); page.wait_for_load_state("networkidle")
        shot(page, "admin-site-required", full=False)
        page.goto(BASE + "/admin/lookups/contractors")
        page.fill("#f_name", f"مؤسسة الريان {TAG}"); page.fill("#f_contact_name", "خالد"); page.fill("#f_phone", "0555555555")
        page.click("main form button:has-text('إضافة')"); page.wait_for_load_state("networkidle")
        shot(page, "admin-contractors")
        page.goto(BASE + "/admin/users"); shot(page, "admin-users")
        page.fill("#f_name", "مشاهد تجريبي"); page.fill("#f_email", f"viewer{TAG}@kidana.local"); page.select_option("#f_role", "viewer")
        page.fill("#f_password", "viewer-password-1")
        page.click("main form button:has-text('إضافة مستخدم')"); page.wait_for_load_state("networkidle")
        shot(page, "admin-user-created", full=False)
        page.fill("#f_name", "مدقق تجريبي"); page.fill("#f_email", f"insp{TAG}@kidana.local"); page.select_option("#f_role", "inspector")
        page.click("main form button:has-text('إضافة مستخدم')"); page.wait_for_load_state("networkidle")
        shot(page, "admin-user-generated-password", full=False)
        page.goto(BASE + "/admin/checklists"); shot(page, "admin-checklists")
        page.goto(BASE + "/admin/checklists")
        page.locator("a:has-text('إصدار جديد من هذه')").first.click(); page.wait_for_load_state("networkidle")
        shot(page, "admin-checklist-new-version")
        page.fill("input[name=weight_0]", "0")
        page.fill("input[name=code_6]", "Q1"); page.fill("input[name=label_6]", "بند مكرر")
        page.locator("main form button:has-text('حفظ الإصدار')").click(); page.wait_for_load_state("networkidle")
        shot(page, "admin-checklist-errors")
        pp.goto(BASE + "/admin/checklists"); shot(pp, "phone-admin-checklists")
    s_admin()

    @step("account: password page validation")
    def s_account():
        page.goto(BASE + "/account/password")
        page.fill("#f_current", "nope"); page.fill("#f_new", "short"); page.fill("#f_confirm", "x")
        page.click("main form button[type=submit]"); page.wait_for_load_state("networkidle")
        shot(page, "password-errors", full=False)
    s_account()

    @step("errors: 404")
    def s_errors():
        page.goto(BASE + "/work-orders/999999"); shot(page, "404-work-order", full=False)
        page.goto(BASE + "/nope"); shot(page, "404-page", full=False)
    s_errors()

    @step("viewer role is read-only")
    def s_viewer():
        vctx = browser.new_context(viewport={"width": 1366, "height": 900}, locale="ar-SA")
        vp = login(vctx, f"viewer{TAG}@kidana.local", "viewer-password-1", "viewer")
        shot(vp, "viewer-dashboard", full=False)
        vp.goto(state["wo_url"]); shot(vp, "viewer-work-order")
        vp.goto(BASE + "/data"); shot(vp, "viewer-data-export-only", full=False)
        vp.goto(BASE + "/work-orders/new"); shot(vp, "viewer-403", full=False)
        vp.goto(BASE + "/admin"); shot(vp, "viewer-admin-403", full=False)
        vctx.close()
    s_viewer()

    @step("user menu + logout")
    def s_logout():
        page.goto(BASE + "/")
        page.locator("header button[aria-haspopup]").click(); page.wait_for_timeout(300)
        shot(page, "user-menu", full=False)
        page.click("text=تسجيل الخروج"); page.wait_for_load_state("networkidle")
        shot(page, "after-logout", full=False)
    s_logout()

    browser.close()

print("\n==== FAILED STEPS ====")
print("\n".join(failures) if failures else "none")
print("==== CONSOLE / NETWORK PROBLEMS ====")
print("\n".join(problems) if problems else "none")
