DELETE FROM tenant WHERE id > 100;
DELETE FROM visit_history WHERE created_at > '2022-05-31 23:59:59';
UPDATE id_generator SET id=2678400000 WHERE stub='a';
ALTER TABLE id_generator AUTO_INCREMENT=2678400000;
