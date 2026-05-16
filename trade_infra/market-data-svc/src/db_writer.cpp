#include "db_writer.h"
#include <stdexcept>

DbWriter::DbWriter(const std::string& connstr) {
    conn_ = PQconnectdb(connstr.c_str());
    if (PQstatus(conn_) != CONNECTION_OK) {
        std::string err = PQerrorMessage(conn_);
        PQfinish(conn_);
        throw std::runtime_error("DB connect failed: " + err);
    }
}
DbWriter::~DbWriter() { if (conn_) PQfinish(conn_); }

void DbWriter::write_tick(const std::string& node, double lmp, double load_mw) {
    std::string lmp_s  = std::to_string(lmp);
    std::string load_s = std::to_string(load_mw);
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
    // NOTIFY downstream listeners with JSON payload
    std::string payload = "{\"node\":\"" + node + "\",\"lmp\":" + lmp_s + "}";
    PGresult* nr = PQexec(conn_, ("NOTIFY price_ticks, '" + payload + "'").c_str());
    PQclear(nr);
}
