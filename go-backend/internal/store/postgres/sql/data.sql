INSERT INTO "user" (id, "user", pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
VALUES (1, 'admin_user', '3c85cdebade1c51cf64ca9f3c09d182d', 0, 2727251700000, 99999, 0, 0, 1, 99999, 1748914865000, 1754011744252, 1)
ON CONFLICT DO NOTHING;

INSERT INTO vite_config (id, name, value, time)
VALUES (1, 'app_name', 'flux', 1755147963000)
ON CONFLICT DO NOTHING;

DO $$
BEGIN
    IF to_regclass('public.user_id_seq') IS NOT NULL THEN
        PERFORM setval('user_id_seq', (SELECT COALESCE(MAX(id), 0) FROM "user"));
    END IF;
    IF to_regclass('public.vite_config_id_seq') IS NOT NULL THEN
        PERFORM setval('vite_config_id_seq', (SELECT COALESCE(MAX(id), 0) FROM vite_config));
    END IF;
END
$$;
