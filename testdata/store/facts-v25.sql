-- Synthetic downgrade fixture for tests of migrations preceding fact sets. It also undoes v28
-- (file lists of fact sets, snapshot blocks): use it on new, empty databases only, since a
-- snapshot's block cannot be turned back into rows here.
DROP TABLE snapshot_list;
CREATE TABLE content_state_legacy AS SELECT instance_id,game_mm,content_key,folders,def_files,patch_files,parse_errors FROM content_state;
CREATE TABLE def_fact_legacy AS SELECT c.instance_id,f.def_type,f.def_name,f.name,f.parent,f.abstract,json_extract(x.files,'$['||f.file_ix||']') AS file,f.line,f.may_require,f.may_require_any,c.game_mm FROM def_fact f JOIN fact_set x ON x.id=f.set_id JOIN content_state c ON c.def_set_id=f.set_id;
CREATE TABLE assembly_fact_legacy AS SELECT c.instance_id,json_extract(x.files,'$['||f.file_ix||']') AS file,f.skipped,f.manual,f.uses_harmony,f.types,f.patches,f.error,c.game_mm FROM assembly_fact f JOIN fact_set x ON x.id=f.set_id JOIN content_state c ON c.dll_set_id=f.set_id;
CREATE TABLE type_fact_legacy AS SELECT c.instance_id,json_extract(x.files,'$['||f.file_ix||']') AS file,f.type_name,c.game_mm FROM type_fact f JOIN fact_set x ON x.id=f.set_id JOIN content_state c ON c.dll_set_id=f.set_id;
CREATE TABLE harmony_fact_legacy AS SELECT c.instance_id,json_extract(x.files,'$['||f.file_ix||']') AS file,f.patch_type,f.patch_method,f.kind,f.target,f.target_type,f.target_method,f.args,f.skips,f.dynamic,c.game_mm FROM harmony_fact f JOIN fact_set x ON x.id=f.set_id JOIN content_state c ON c.dll_set_id=f.set_id;
CREATE TABLE harmony_id_legacy AS SELECT c.instance_id,json_extract(x.files,'$['||f.file_ix||']') AS file,f.id,c.game_mm FROM harmony_id f JOIN fact_set x ON x.id=f.set_id JOIN content_state c ON c.dll_set_id=f.set_id;
DROP TABLE content_state;
DROP TABLE def_fact;
DROP TABLE assembly_fact;
DROP TABLE type_fact;
DROP TABLE harmony_fact;
DROP TABLE harmony_id;
DROP TABLE fact_set;
ALTER TABLE content_state_legacy RENAME TO content_state;
CREATE UNIQUE INDEX content_state_pk ON content_state(instance_id,game_mm);
ALTER TABLE def_fact_legacy RENAME TO def_fact;
CREATE INDEX def_fact_inst_mm ON def_fact(instance_id,game_mm);
ALTER TABLE assembly_fact_legacy RENAME TO assembly_fact;
CREATE INDEX assembly_fact_inst_mm ON assembly_fact(instance_id,game_mm);
ALTER TABLE type_fact_legacy RENAME TO type_fact;
CREATE INDEX type_fact_inst_mm ON type_fact(instance_id,game_mm);
ALTER TABLE harmony_fact_legacy RENAME TO harmony_fact;
CREATE INDEX harmony_fact_inst_mm ON harmony_fact(instance_id,game_mm);
ALTER TABLE harmony_id_legacy RENAME TO harmony_id;
CREATE INDEX harmony_id_inst_mm ON harmony_id(instance_id,game_mm);
