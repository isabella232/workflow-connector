#!/bin/sh

cat << __EOF__ >> ~/migration.psql
BEGIN;

CREATE TABLE IF NOT EXISTS teacher (
id integer not null primary key,
code char(6) unique not null);

CREATE TABLE IF NOT EXISTS organization (
id integer not null primary key,
code varchar(8) unique not null);

CREATE TABLE IF NOT EXISTS section (
id integer not null primary key,
code char(1) unique not null);

CREATE TABLE IF NOT EXISTS course (
id integer not null primary key,
code char(4) unique not null);

CREATE TABLE IF NOT EXISTS class (
id integer not null primary key,
section_code char(1) references section(code),
course_code char(4) references course(code),
organization_code varchar(8) references organization(code));

CREATE TABLE IF NOT EXISTS class_teacher (
id integer not null primary key,
class_id integer references class(id),
teacher_id char(6) references teacher(code));


COMMIT;
__EOF__
DIR=$(pwd)/build
psql ${DATABASE_URL} -f migration.psql
for table in teacher organization section course class; do
    psql ${DATABASE_URL} -c "\copy ${table} FROM '${DIR}/${table}-data.csv' DELIMITER ',' CSV"
done
