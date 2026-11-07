import Foundation

public enum NoonmarkError: Error, Equatable, Sendable {
    case notFound(String)
    case invalidTitle
    case chainAbandoned
    case immutableHistory
    case activeTraceAlreadyExists
    case noActiveTrace
    case invalidInput(String)
    case invalidTransition(String)
    case lockedDay
    case futurePlanCannotComplete
    case historicalCompletionCannotBeUndone
    case openSubtasksPreventCompletion
}

extension NoonmarkError: LocalizedError {
    public var errorDescription: String? {
        switch self {
        case let .notFound(resource):
            "找不到\(resource)。"
        case .invalidTitle:
            "任务标题不能为空。"
        case .chainAbandoned:
            "已废弃的任务不能执行此操作。"
        case .immutableHistory:
            "历史记录不可改写。"
        case .activeTraceAlreadyExists:
            "这项任务已有进行中的轨迹。"
        case .noActiveTrace:
            "这项任务没有进行中的轨迹。"
        case let .invalidInput(message), let .invalidTransition(message):
            message
        case .lockedDay:
            "这个日期已经锁定。"
        case .futurePlanCannotComplete:
            "未来任务不能提前标记完成。"
        case .historicalCompletionCannotBeUndone:
            "历史完成记录不能撤回。"
        case .openSubtasksPreventCompletion:
            "还有未完成子任务，完成全部子任务后才能完成父任务"
        }
    }
}
