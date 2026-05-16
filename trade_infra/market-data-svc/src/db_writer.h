#pragma once
#include <string>
#include <libpq-fe.h>

class DbWriter {
public:
    explicit DbWriter(const std::string& connstr);
    ~DbWriter();
    void write_tick(const std::string& node, double lmp, double load_mw);
private:
    PGconn* conn_;
};
