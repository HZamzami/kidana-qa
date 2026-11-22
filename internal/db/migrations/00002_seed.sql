-- +goose Up
-- Reference data: Kidana sites and systems, and the first QA and QC checklists.

INSERT INTO sites (name, sort) VALUES
    ('منى', 1), ('عرفات', 2), ('مزدلفة', 3), ('مكة المكرمة', 4), ('المقر الرئيسي', 5);

INSERT INTO systems (name, sort) VALUES
    ('التكييف والتبريد', 1), ('الكهرباء', 2), ('المولدات', 3), ('السباكة والصرف', 4),
    ('مكافحة الحريق', 5), ('المصاعد', 6), ('الإنارة', 7), ('الأعمال المدنية', 8),
    ('أنظمة التيار الخفيف', 9);

-- QA: during the work — how the contractor works.
WITH t AS (
    INSERT INTO checklist_templates (stage, version, name, active)
    VALUES ('qa', 1, 'رقابة أثناء التنفيذ — الإصدار 1', true) RETURNING id
)
INSERT INTO template_criteria (template_id, code, label, description, weight, is_critical, sort)
SELECT t.id, c.code, c.label, c.descr, c.weight, c.crit, c.sort FROM t, (VALUES
    ('Q1', 'اشتراطات الصحة والسلامة',
     'استخدام معدات الوقاية الشخصية، تطبيق العزل والإقفال (LOTO)، ووجود تصاريح العمل عند الحاجة.', 30, true, 1),
    ('Q2', 'الالتزام بالإجراء وقائمة الفحص',
     'تنفيذ العمل وفق الإجراء المعتمد وقائمة الفحص الخاصة بالأصل دون حذف أو اختصار.', 20, false, 2),
    ('Q3', 'المواد وقطع الغيار المعتمدة',
     'استخدام مواد وقطع غيار مطابقة للمواصفات ومعتمدة، وتسجيل المصروف منها.', 20, false, 3),
    ('Q4', 'كفاءة الفني والعدد المناسبة',
     'الفني مؤهل للعمل المطلوب، ويستخدم العدد وأجهزة القياس المناسبة والمعايرة.', 15, false, 4),
    ('Q5', 'الحضور وزمن الاستجابة',
     'الوصول في الموعد المجدول للصيانة الوقائية، أو ضمن زمن الاستجابة (SLA) لأمر العمل حسب الأولوية.', 15, false, 5)
) AS c(code, label, descr, weight, crit, sort);

-- QC: after the work — is the result correct.
WITH t AS (
    INSERT INTO checklist_templates (stage, version, name, active)
    VALUES ('qc', 1, 'فحص بعد التنفيذ — الإصدار 1', true) RETURNING id
)
INSERT INTO template_criteria (template_id, code, label, description, weight, is_critical, sort)
SELECT t.id, c.code, c.label, c.descr, c.weight, c.crit, c.sort FROM t, (VALUES
    ('C1', 'جودة التنفيذ الفني',
     'مطابقة العمل للمواصفات الفنية وتوصيات المصنّع، وعودة الأصل للعمل بكفاءة دون تكرار العطل.', 35, false, 1),
    ('C2', 'اكتمال قائمة الفحص والقراءات',
     'تعبئة جميع بنود قائمة الفحص وتسجيل القراءات الفنية كاملة.', 15, false, 2),
    ('C3', 'التوثيق والصور',
     'إرفاق صور قبل/بعد، وتسجيل الملاحظات الفنية في النظام بدقة.', 15, false, 3),
    ('C4', 'نظافة الموقع بعد العمل',
     'إزالة المخلفات وإعادة الموقع لحالته الطبيعية وتأمينه بعد الانتهاء.', 10, false, 4),
    ('C5', 'إغلاق أمر العمل في النظام',
     'إغلاق أمر العمل في نظام إدارة الصيانة بشكل صحيح ودقيق.', 10, false, 5),
    ('C6', 'الإنجاز في الموعد',
     'إنجاز العمل في موعده المجدول أو ضمن زمن SLA المعتمد.', 15, false, 6)
) AS c(code, label, descr, weight, crit, sort);

-- +goose Down
DELETE FROM template_criteria;
DELETE FROM checklist_templates;
DELETE FROM systems;
DELETE FROM sites;
