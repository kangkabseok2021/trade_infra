#include "db_writer.h"
#include <stdexcept>
#include <cstdio>

DbWriter::DbWriter(const std::string& connstr) {
    conn_ = PQconnectdb(connstr.c_str());
    if (PQstatus(conn_) != CONNECTION_OK) {
        std::string err = PQerrorMessage(conn_);
        PQfinish(conn_);
        throw std::runtime_error("DB connect failed: " + err);
    }
}
DbWriter::~DbWriter() { if (conn_) PQfinish(conn_); }

// Format a double with locale-independent decimal point.
static std::string fmt_double(double v) {
    char buf[64];
    std::snprintf(buf, sizeof(buf), "%.6g", v);
    return buf;
}

void DbWriter::write_tick(const std::string& node, double lmp, double load_mw) {
    std::string lmp_s  = fmt_double(lmp);
    std::string load_s = fmt_double(load_mw);
    const char* params[] = { node.c_str(), lmp_s.c_str(), load_s.c_str() };
    PGresult* r = PQexecParams(conn_,
        "INSERT INTO price_ticks (node, lmp, load_mw) VALUES ($1,$2::numeric,$3::numeric)",
        3, nullptr, params, nullptr, nullptr, 0);
    if (PQresultStatus(r) != PGRES_COMMAND_OK) {
        std::string err = PQerrorMessage(conn_);
        PQclear(r);
        throw std::runtime_error("INSERT failed: " + err);
    }
    PQclear(r);
    // NOTIFY via pg_notify($1,$2) to avoid SQL injection through node/lmp values.
    std::string payload = "{\"node\":\"" + node + "\",\"lmp\":" + lmp_s + "}";
    const char* nparams[] = { "price_ticks", payload.c_str() };
    PGresult* nr = PQexecParams(conn_,
        "SELECT pg_notify($1, $2)",
        2, nullptr, nparams, nullptr, nullptr, 0);
    PQclear(nr);
}
