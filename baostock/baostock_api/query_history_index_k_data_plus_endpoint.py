from __future__ import annotations

from dataclasses import dataclass

from .request import get_optional_query_param, get_required_query_param
from .shared import BaostockQueryError, baostock_lock, bs


@dataclass(frozen=True)
class QueryHistoryIndexKDataPlusParams:
    code: str
    fields: str
    start_date: str | None
    end_date: str | None
    frequency: str | None


@dataclass(frozen=True)
class QueryHistoryIndexKDataPlusResult:
    code: str
    fields: list[str]
    start_date: str | None
    end_date: str | None
    frequency: str | None
    total: int
    items: list[dict[str, str]]

    def to_dict(self) -> dict[str, object]:
        return {
            "code": self.code,
            "fields": self.fields,
            "start_date": self.start_date,
            "end_date": self.end_date,
            "frequency": self.frequency,
            "total": self.total,
            "items": self.items,
        }


def parse_params(query: dict[str, list[str]]) -> QueryHistoryIndexKDataPlusParams:
    return QueryHistoryIndexKDataPlusParams(
        code=get_required_query_param(query, "code"),
        fields=get_required_query_param(query, "fields"),
        start_date=get_optional_query_param(query, "start_date"),
        end_date=get_optional_query_param(query, "end_date"),
        frequency=get_optional_query_param(query, "frequency"),
    )


def execute(params: QueryHistoryIndexKDataPlusParams) -> dict[str, object]:
    query_kwargs = {
        "code": params.code,
        "fields": params.fields,
    }

    if params.start_date is not None:
        query_kwargs["start_date"] = params.start_date
    if params.end_date is not None:
        query_kwargs["end_date"] = params.end_date
    if params.frequency is not None:
        query_kwargs["frequency"] = params.frequency

    with baostock_lock:
        login_result = bs.login()

        if login_result.error_code != "0":
            raise RuntimeError(f"baostock login failed: {login_result.error_msg}")

        try:
            result = bs.query_history_k_data_plus(**query_kwargs)

            if result.error_code != "0":
                raise BaostockQueryError(
                    result.error_msg or "query_history_index_k_data_plus failed"
                )

            items: list[dict[str, str]] = []
            while result.next():
                items.append(dict(zip(result.fields, result.get_row_data())))
        finally:
            bs.logout()

    payload = QueryHistoryIndexKDataPlusResult(
        code=params.code,
        fields=list(result.fields),
        start_date=params.start_date,
        end_date=params.end_date,
        frequency=params.frequency,
        total=len(items),
        items=items,
    )
    return payload.to_dict()
