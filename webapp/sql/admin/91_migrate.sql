INSERT INTO visit_history_s AS SELECT player_name, tenant_id, competition_id, min(created_at) as min_created_at
   FROM visit_history group by player_name, tenant_id, competition_id;
